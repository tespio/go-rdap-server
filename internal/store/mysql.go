package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/rdap"
)

// MySQLStore is a MySQL-backed implementation of the storage interface.
// The MySQL schema is provided in migrations/002_mysql_init.sql and
// examples/mysql/schema.sql.
type MySQLStore struct {
	db *sql.DB
}

// NewMySQLStore opens a connection pool against MySQL. The DSN can be either the
// native go-sql-driver format
//
//	user:password@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4
//
// or the URL form (accepted for consistency with PostgreSQL's postgres:// DSN)
//
//	mysql://user:password@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4
func NewMySQLStore(cfg config.StorageConfig) (*MySQLStore, error) {
	dsn, err := normalizeMySQLDSN(cfg.DSN)
	if err != nil {
		return nil, err
	}

	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse mysql DSN: %w", err)
	}
	// Required so time.Time columns scan correctly.
	mc.ParseTime = true
	if mc.Collation == "" {
		mc.Collation = "utf8mb4_general_ci"
	}

	db, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	if cfg.MaxOpen > 0 {
		db.SetMaxOpenConns(cfg.MaxOpen)
	}
	if cfg.MaxIdle > 0 {
		db.SetMaxIdleConns(cfg.MaxIdle)
	}
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &MySQLStore{db: db}, nil
}

// normalizeMySQLDSN accepts both the native go-sql-driver DSN and a mysql:// URL.
// The URL form supports the same query parameters as the native DSN
// (parseTime, charset, collation, tls, timeout, readTimeout, writeTimeout, loc, ...);
// unknown parameters are passed through verbatim.
func normalizeMySQLDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "mysql://") {
		return dsn, nil
	}

	// net/url rejects the go-sql-driver style host mysql://user@tcp(host:port)/db,
	// so rewrite it to the plain mysql://user@host:port/db form first.
	pre := dsn
	if idx := strings.Index(pre, "@tcp("); idx >= 0 {
		rest := pre[idx+len("@tcp("):]
		if i := strings.Index(rest, ")"); i >= 0 {
			pre = pre[:idx+len("@")] + rest[:i] + rest[i+1:]
		}
	} else if strings.HasPrefix(pre[len("mysql://"):], "tcp(") {
		rest := pre[len("mysql://")+len("tcp("):]
		if i := strings.Index(rest, ")"); i >= 0 {
			pre = pre[:len("mysql://")] + rest[:i] + rest[i+1:]
		}
	}

	u, err := url.Parse(pre)
	if err != nil {
		return "", fmt.Errorf("parse mysql URL %q: %w", dsn, err)
	}

	mc := mysql.NewConfig()
	mc.Net = "tcp"
	mc.Params = make(map[string]string)
	if u.User != nil {
		mc.User = u.User.Username()
		mc.Passwd, _ = u.User.Password()
	}

	mc.Addr = u.Host
	mc.DBName = strings.TrimPrefix(u.Path, "/")

	for k, v := range u.Query() {
		switch k {
		case "parseTime":
			mc.ParseTime = len(v) > 0 && v[0] == "true"
		case "charset":
			mc.Params["charset"] = v[0]
		case "collation":
			mc.Collation = v[0]
		default:
			// timeout, readTimeout, writeTimeout, tls, loc, etc. are parsed by
			// mysql.ParseDSN from the formatted DSN.
			mc.Params[k] = v[0]
		}
	}

	return mc.FormatDSN(), nil
}

func (s *MySQLStore) LookupDomain(name string) (*rdap.DomainRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
		FROM domains WHERE ldh_name = ? OR unicode_name = ?
	`

	var record rdap.DomainRecord
	var statusJSON, nsJSON, sdJSON []byte

	err := s.db.QueryRowContext(context.Background(), query, name, name).Scan(
		&record.Handle, &record.LDHName, &record.UnicodeName, &record.TLD,
		&statusJSON, &record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt,
		&record.Registrant, &record.Admin, &record.Tech, &record.Billing,
		&nsJSON, &sdJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("domain lookup: %w", err)
	}

	json.Unmarshal(statusJSON, &record.Status)
	json.Unmarshal(nsJSON, &record.Nameservers)
	if sdJSON != nil {
		json.Unmarshal(sdJSON, &record.SecureDNS)
	}

	return &record, nil
}

func (s *MySQLStore) LookupEntity(handle string) (*rdap.EntityRecord, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities WHERE handle = ?
	`

	var record rdap.EntityRecord
	var rolesJSON, statusJSON, pidJSON []byte

	err := s.db.QueryRowContext(context.Background(), query, handle).Scan(
		&record.Handle, &record.VCardJSON, &rolesJSON, &statusJSON,
		&record.CreatedAt, &record.UpdatedAt, &pidJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("entity lookup: %w", err)
	}

	json.Unmarshal(rolesJSON, &record.Roles)
	json.Unmarshal(statusJSON, &record.Status)
	json.Unmarshal(pidJSON, &record.PublicIDs)

	return &record, nil
}

func (s *MySQLStore) LookupNameserver(name string) (*rdap.NameserverRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers WHERE ldh_name = ? OR unicode_name = ?
	`

	var record rdap.NameserverRecord
	var ipv4JSON, ipv6JSON, statusJSON []byte

	err := s.db.QueryRowContext(context.Background(), query, name, name).Scan(
		&record.Handle, &record.LDHName, &record.UnicodeName,
		&ipv4JSON, &ipv6JSON, &statusJSON,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("nameserver lookup: %w", err)
	}

	json.Unmarshal(ipv4JSON, &record.IPV4)
	json.Unmarshal(ipv6JSON, &record.IPV6)
	json.Unmarshal(statusJSON, &record.Status)

	return &record, nil
}

func (s *MySQLStore) LookupIPNetwork(cidr string) (*rdap.IPNetworkRecord, error) {
	version, start4, end4, start6, end6, err := ipRange(cidr)
	if err != nil {
		return nil, err
	}

	var query string
	var args []any
	if version == "v4" {
		query = `
			SELECT handle, start_address, end_address, ip_version, cidr,
			       name, type, country, status, created_at, updated_at
			FROM ip_networks
			WHERE ip_version = 'v4' AND start_ip <= ? AND end_ip >= ?
			LIMIT 1
		`
		args = []any{start4, end4}
	} else {
		query = `
			SELECT handle, start_address, end_address, ip_version, cidr,
			       name, type, country, status, created_at, updated_at
			FROM ip_networks
			WHERE ip_version = 'v6' AND start_ip6 <= ? AND end_ip6 >= ?
			LIMIT 1
		`
		args = []any{start6, end6}
	}

	var record rdap.IPNetworkRecord
	var cidrJSON, statusJSON []byte

	err = s.db.QueryRowContext(context.Background(), query, args...).Scan(
		&record.Handle, &record.StartAddress, &record.EndAddress,
		&record.IPVersion, &cidrJSON, &record.Name, &record.Type,
		&record.Country, &statusJSON, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("IP network lookup: %w", err)
	}

	json.Unmarshal(cidrJSON, &record.CIDR)
	json.Unmarshal(statusJSON, &record.Status)

	return &record, nil
}

func (s *MySQLStore) SearchDomainsByName(pattern string, limit int) ([]rdap.DomainRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing
		FROM domains
		WHERE ldh_name LIKE ? OR unicode_name LIKE ?
		LIMIT ?
	`

	sqlPattern := patternToSQL(pattern)
	rows, err := s.db.QueryContext(context.Background(), query, sqlPattern, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search domains: %w", err)
	}
	defer rows.Close()

	return scanDomainRows(rows)
}

func (s *MySQLStore) SearchDomainsByNS(nsName string, limit int) ([]rdap.DomainRecord, error) {
	query := `
		SELECT d.handle, d.ldh_name, d.unicode_name, d.tld, d.status,
		       d.created_at, d.updated_at, d.expires_at,
		       d.registrant, d.admin, d.tech, d.billing
		FROM domains d
		JOIN domain_nameservers dn ON d.handle = dn.domain_handle
		JOIN nameservers n ON dn.ns_handle = n.handle
		WHERE n.ldh_name = ? OR n.unicode_name = ?
		LIMIT ?
	`

	rows, err := s.db.QueryContext(context.Background(), query, nsName, nsName, limit)
	if err != nil {
		return nil, fmt.Errorf("search domains by NS: %w", err)
	}
	defer rows.Close()

	return scanDomainRows(rows)
}

func (s *MySQLStore) SearchEntitiesByName(pattern string, limit int) ([]rdap.EntityRecord, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities
		WHERE vcard_json LIKE ? OR handle LIKE ?
		LIMIT ?
	`

	sqlPattern := "%" + pattern + "%"
	rows, err := s.db.QueryContext(context.Background(), query, sqlPattern, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	defer rows.Close()

	return scanEntityRows(rows)
}

func (s *MySQLStore) SearchEntitiesByHandle(pattern string, limit int) ([]rdap.EntityRecord, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities
		WHERE handle LIKE ?
		LIMIT ?
	`

	sqlPattern := patternToSQL(pattern)
	rows, err := s.db.QueryContext(context.Background(), query, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search entities by handle: %w", err)
	}
	defer rows.Close()

	return scanEntityRows(rows)
}

func (s *MySQLStore) SearchNameserversByName(pattern string, limit int) ([]rdap.NameserverRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers
		WHERE ldh_name LIKE ? OR unicode_name LIKE ?
		LIMIT ?
	`

	sqlPattern := patternToSQL(pattern)
	rows, err := s.db.QueryContext(context.Background(), query, sqlPattern, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search nameservers: %w", err)
	}
	defer rows.Close()

	return scanNameserverRows(rows)
}

func (s *MySQLStore) SearchNameserversByIP(ip string, limit int) ([]rdap.NameserverRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers
		WHERE JSON_CONTAINS(ipv4, ?) OR JSON_CONTAINS(ipv6, ?)
		LIMIT ?
	`

	ipJSON := fmt.Sprintf(`["%s"]`, ip)
	rows, err := s.db.QueryContext(context.Background(), query, ipJSON, ipJSON, limit)
	if err != nil {
		return nil, fmt.Errorf("search nameservers by IP: %w", err)
	}
	defer rows.Close()

	return scanNameserverRows(rows)
}

func (s *MySQLStore) Ping() error {
	return s.db.PingContext(context.Background())
}

func (s *MySQLStore) Close() error {
	return s.db.Close()
}

func scanDomainRows(rows *sql.Rows) ([]rdap.DomainRecord, error) {
	var results []rdap.DomainRecord
	for rows.Next() {
		var record rdap.DomainRecord
		var statusJSON []byte
		if err := rows.Scan(
			&record.Handle, &record.LDHName, &record.UnicodeName, &record.TLD,
			&statusJSON, &record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt,
			&record.Registrant, &record.Admin, &record.Tech, &record.Billing,
		); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		json.Unmarshal(statusJSON, &record.Status)
		results = append(results, record)
	}
	return results, rows.Err()
}

func scanEntityRows(rows *sql.Rows) ([]rdap.EntityRecord, error) {
	var results []rdap.EntityRecord
	for rows.Next() {
		var record rdap.EntityRecord
		var rolesJSON, statusJSON, pidJSON []byte
		if err := rows.Scan(
			&record.Handle, &record.VCardJSON, &rolesJSON, &statusJSON,
			&record.CreatedAt, &record.UpdatedAt, &pidJSON,
		); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		json.Unmarshal(rolesJSON, &record.Roles)
		json.Unmarshal(statusJSON, &record.Status)
		json.Unmarshal(pidJSON, &record.PublicIDs)
		results = append(results, record)
	}
	return results, rows.Err()
}

func scanNameserverRows(rows *sql.Rows) ([]rdap.NameserverRecord, error) {
	var results []rdap.NameserverRecord
	for rows.Next() {
		var record rdap.NameserverRecord
		var ipv4JSON, ipv6JSON, statusJSON []byte
		if err := rows.Scan(
			&record.Handle, &record.LDHName, &record.UnicodeName,
			&ipv4JSON, &ipv6JSON, &statusJSON,
			&record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan nameserver: %w", err)
		}
		json.Unmarshal(ipv4JSON, &record.IPV4)
		json.Unmarshal(ipv6JSON, &record.IPV6)
		json.Unmarshal(statusJSON, &record.Status)
		results = append(results, record)
	}
	return results, rows.Err()
}

// ipRange computes the numeric range covered by a CIDR so it can be matched
// against stored numeric IP columns (MySQL has no native CIDR type).
func ipRange(cidr string) (version string, start4, end4 uint64, start6, end6 []byte, err error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return "", 0, 0, nil, nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	prefix = prefix.Masked()
	addr := prefix.Addr()

	if addr.Is4() {
		a := addr.As4()
		start := uint64(binary.BigEndian.Uint32(a[:]))
		hostBits := 32 - prefix.Bits()
		end := start + (uint64(1) << hostBits) - 1
		return "v4", start, end, nil, nil, nil
	}

	a := addr.As16()
	start := make([]byte, 16)
	copy(start, a[:])
	end := make([]byte, 16)
	copy(end, start)
	hostBits := 128 - prefix.Bits()
	for i := 0; i < hostBits; i++ {
		byteIdx := 15 - i/8
		bitIdx := uint(i % 8)
		end[byteIdx] |= 1 << bitIdx
	}
	return "v6", 0, 0, start, end, nil
}