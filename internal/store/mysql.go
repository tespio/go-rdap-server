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
	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
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
func normalizeMySQLDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "mysql://") {
		return dsn, nil
	}

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
			mc.Params[k] = v[0]
		}
	}

	return mc.FormatDSN(), nil
}

func (s *MySQLStore) LookupDomain(name string) (*domain.Domain, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
		FROM domains WHERE ldh_name = ? OR unicode_name = ?
	`

	var record domain.Domain
	var statusJSON, nsJSON, sdJSON []byte
	var created, updated, expires time.Time
	var registrant, admin, tech, billing *string

	err := s.db.QueryRowContext(context.Background(), query, name, name).Scan(
		&record.Handle, &record.LDHName, &record.UnicodeName, &record.TLD,
		&statusJSON, &created, &updated, &expires,
		&registrant, &admin, &tech, &billing,
		&nsJSON, &sdJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("domain lookup: %w", err)
	}

	record.Status = parseStatus(statusJSON)
	record.ExpiresAt = expires
	record.SecureDNS = parseSecureDNS(sdJSON)
	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}
	populateDomainContacts(&record, registrant, admin, tech, billing)

	if len(nsJSON) > 0 {
		record.Nameservers = parseNameservers(nsJSON)
	}

	return &record, nil
}

// GetDomainAggregate resolves a domain plus its registrar, contacts, and
// nameservers within a single REPEATABLE READ transaction. All related objects
// are read from the same snapshot, so a concurrent update (e.g. a registrar
// transfer, nameserver change, or renewal) can never produce a torn response.
func (s *MySQLStore) GetDomainAggregate(name string) (*domain.DomainAggregate, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
		FROM domains WHERE ldh_name = ? OR unicode_name = ?
	`, name, name)
	d, err := scanMySQLDomainRow(row)
	if err != nil {
		return nil, fmt.Errorf("domain lookup: %w", err)
	}

	agg := &domain.DomainAggregate{
		Domain:      d,
		Contacts:    map[string]*domain.Contact{},
		Nameservers: map[string]*domain.NameServer{},
	}

	handles := make(map[string]bool)
	if d.Registrar != "" {
		handles[d.Registrar] = true
	}
	for _, hs := range d.Contacts {
		for _, h := range hs {
			handles[h] = true
		}
	}
	for h := range handles {
		contact, err := s.lookupContactTx(ctx, tx, h)
		if err == nil {
			agg.Contacts[h] = contact
			if h == d.Registrar {
				agg.Registrar = contact
			}
		}
	}

	for _, ns := range d.Nameservers {
		nsRow := tx.QueryRowContext(ctx, `
			SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
			FROM nameservers WHERE handle = ?
		`, ns.Handle)
		if n, err := scanMySQLNameserverRow(nsRow); err == nil {
			agg.Nameservers[n.Handle] = n
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return agg, nil
}

func (s *MySQLStore) lookupContactTx(ctx context.Context, tx *sql.Tx, handle string) (*domain.Contact, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities WHERE handle = ?
	`, handle)
	return scanMySQLContactRow(row)
}

func (s *MySQLStore) LookupContact(handle string) (*domain.Contact, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities WHERE handle = ?
	`

	var record domain.Contact
	var rolesJSON, statusJSON, pidJSON []byte
	var vcardJSON *string
	var created, updated time.Time

	err := s.db.QueryRowContext(context.Background(), query, handle).Scan(
		&record.Handle, &vcardJSON, &rolesJSON, &statusJSON,
		&created, &updated, &pidJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("entity lookup: %w", err)
	}

	var roles []string
	json.Unmarshal(rolesJSON, &roles)
	for _, r := range roles {
		record.Roles = append(record.Roles, domain.ContactRole(r))
	}
	record.Status = parseStatus(statusJSON)

	var pids []domain.PublicID
	json.Unmarshal(pidJSON, &pids)
	record.PublicIDs = pids

	if vcardJSON != nil && *vcardJSON != "" {
		record.VCard = parseVCardJSON(*vcardJSON)
	}

	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}

	return &record, nil
}

func (s *MySQLStore) LookupNameserver(name string) (*domain.NameServer, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers WHERE ldh_name = ? OR unicode_name = ?
	`

	var record domain.NameServer
	var ipv4JSON, ipv6JSON, statusJSON []byte
	var created, updated time.Time

	err := s.db.QueryRowContext(context.Background(), query, name, name).Scan(
		&record.Handle, &record.LDHName, &record.UnicodeName,
		&ipv4JSON, &ipv6JSON, &statusJSON,
		&created, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("nameserver lookup: %w", err)
	}

	json.Unmarshal(ipv4JSON, &record.IPV4)
	json.Unmarshal(ipv6JSON, &record.IPV6)
	record.Status = parseStatus(statusJSON)
	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}

	return &record, nil
}

func (s *MySQLStore) LookupIPNetwork(cidr string) (*domain.IPNetwork, error) {
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

	var record domain.IPNetwork
	var cidrJSON, statusJSON []byte
	var created, updated time.Time

	err = s.db.QueryRowContext(context.Background(), query, args...).Scan(
		&record.Handle, &record.StartAddress, &record.EndAddress,
		&record.IPVersion, &cidrJSON, &record.Name, &record.Type,
		&record.Country, &statusJSON, &created, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("IP network lookup: %w", err)
	}

	json.Unmarshal(cidrJSON, &record.CIDR)
	record.Status = parseStatus(statusJSON)
	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}

	return &record, nil
}

func (s *MySQLStore) LookupAutnum(asn int) (*domain.Autnum, error) {
	query := `
		SELECT handle, start_asn, end_asn, name, type, country, status, created_at, updated_at
		FROM autnums WHERE start_asn <= ? AND end_asn >= ?
		LIMIT 1
	`

	var record domain.Autnum
	var statusJSON []byte
	var created, updated time.Time

	err := s.db.QueryRowContext(context.Background(), query, asn, asn).Scan(
		&record.Handle, &record.StartASN, &record.EndASN,
		&record.Name, &record.Type, &record.Country,
		&statusJSON, &created, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("autnum lookup: %w", err)
	}

	record.Status = parseStatus(statusJSON)
	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}

	return &record, nil
}

func (s *MySQLStore) SearchDomainsByName(pattern string, limit int) ([]domain.Domain, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
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

	return scanMySQLDomainRows(rows)
}

func (s *MySQLStore) SearchDomainsByNS(nsName string, limit int) ([]domain.Domain, error) {
	query := `
		SELECT d.handle, d.ldh_name, d.unicode_name, d.tld, d.status,
		       d.created_at, d.updated_at, d.expires_at,
		       d.registrant, d.admin, d.tech, d.billing,
		       d.nameservers, d.secure_dns
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

	return scanMySQLDomainRows(rows)
}

func (s *MySQLStore) SearchContactsByName(pattern string, limit int) ([]domain.Contact, error) {
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

	return scanMySQLContactRows(rows)
}

func (s *MySQLStore) SearchContactsByHandle(pattern string, limit int) ([]domain.Contact, error) {
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

	return scanMySQLContactRows(rows)
}

func (s *MySQLStore) SearchNameserversByName(pattern string, limit int) ([]domain.NameServer, error) {
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

	return scanMySQLNameserverRows(rows)
}

func (s *MySQLStore) SearchNameserversByIP(ip string, limit int) ([]domain.NameServer, error) {
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

	return scanMySQLNameserverRows(rows)
}

func (s *MySQLStore) Ping() error {
	return s.db.PingContext(context.Background())
}

func (s *MySQLStore) Close() error {
	return s.db.Close()
}

// populateDomainContacts maps registrant/admin/tech/billing columns to the
// domain's Contacts map and sets the registrar reference.
func populateDomainContacts(d *domain.Domain, registrant, admin, tech, billing *string) {
	d.Contacts = map[domain.ContactRole][]string{}
	if registrant != nil {
		d.Contacts[domain.RoleRegistrant] = []string{*registrant}
		d.Registrar = *registrant
	}
	if admin != nil {
		d.Contacts[domain.RoleAdministrative] = []string{*admin}
	}
	if tech != nil {
		d.Contacts[domain.RoleTechnical] = []string{*tech}
	}
	if billing != nil {
		d.Contacts[domain.RoleBilling] = []string{*billing}
	}
}

type mysqlRowScanner interface {
	Scan(dest ...any) error
}

func scanMySQLDomainRow(row mysqlRowScanner) (*domain.Domain, error) {
	var record domain.Domain
	var statusJSON, nsJSON, sdJSON []byte
	var created, updated, expires time.Time
	var registrant, admin, tech, billing *string

	if err := row.Scan(
		&record.Handle, &record.LDHName, &record.UnicodeName, &record.TLD,
		&statusJSON, &created, &updated, &expires,
		&registrant, &admin, &tech, &billing,
		&nsJSON, &sdJSON,
	); err != nil {
		return nil, fmt.Errorf("scan domain: %w", err)
	}

	record.Status = parseStatus(statusJSON)
	record.ExpiresAt = expires
	record.SecureDNS = parseSecureDNS(sdJSON)
	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}
	populateDomainContacts(&record, registrant, admin, tech, billing)

	if len(nsJSON) > 0 {
		record.Nameservers = parseNameservers(nsJSON)
	}

	return &record, nil
}

func scanMySQLDomainRows(rows *sql.Rows) ([]domain.Domain, error) {
	var results []domain.Domain
	for rows.Next() {
		d, err := scanMySQLDomainRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *d)
	}
	return results, rows.Err()
}

func scanMySQLContactRow(row mysqlRowScanner) (*domain.Contact, error) {
	var record domain.Contact
	var rolesJSON, statusJSON, pidJSON []byte
	var vcardJSON *string
	var created, updated time.Time

	if err := row.Scan(
		&record.Handle, &vcardJSON, &rolesJSON, &statusJSON,
		&created, &updated, &pidJSON,
	); err != nil {
		return nil, fmt.Errorf("scan entity: %w", err)
	}

	var roles []string
	json.Unmarshal(rolesJSON, &roles)
	for _, r := range roles {
		record.Roles = append(record.Roles, domain.ContactRole(r))
	}
	record.Status = parseStatus(statusJSON)

	var pids []domain.PublicID
	json.Unmarshal(pidJSON, &pids)
	record.PublicIDs = pids

	if vcardJSON != nil && *vcardJSON != "" {
		record.VCard = parseVCardJSON(*vcardJSON)
	}

	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}

	return &record, nil
}

func scanMySQLContactRows(rows *sql.Rows) ([]domain.Contact, error) {
	var results []domain.Contact
	for rows.Next() {
		c, err := scanMySQLContactRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *c)
	}
	return results, rows.Err()
}

func scanMySQLNameserverRow(row mysqlRowScanner) (*domain.NameServer, error) {
	var record domain.NameServer
	var ipv4JSON, ipv6JSON, statusJSON []byte
	var created, updated time.Time

	if err := row.Scan(
		&record.Handle, &record.LDHName, &record.UnicodeName,
		&ipv4JSON, &ipv6JSON, &statusJSON,
		&created, &updated,
	); err != nil {
		return nil, fmt.Errorf("scan nameserver: %w", err)
	}

	json.Unmarshal(ipv4JSON, &record.IPV4)
	json.Unmarshal(ipv6JSON, &record.IPV6)
	record.Status = parseStatus(statusJSON)
	record.Metadata = domain.Metadata{
		Version:   1,
		CreatedAt: created,
		UpdatedAt: updated,
		Source:    "mysql",
	}

	return &record, nil
}

func scanMySQLNameserverRows(rows *sql.Rows) ([]domain.NameServer, error) {
	var results []domain.NameServer
	for rows.Next() {
		n, err := scanMySQLNameserverRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *n)
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
