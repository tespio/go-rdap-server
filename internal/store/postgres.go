package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/rdap"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(cfg config.StorageConfig) (*PostgresStore, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse pgx config: %w", err)
	}

	if cfg.MaxOpen > 0 {
		poolCfg.MaxConns = int32(cfg.MaxOpen)
	}
	if cfg.MaxIdle > 0 {
		poolCfg.MinConns = int32(cfg.MaxIdle)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) LookupDomain(name string) (*rdap.DomainRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
		FROM domains WHERE ldh_name = $1 OR unicode_name = $1
	`

	var record rdap.DomainRecord
	var statusJSON, nsJSON, sdJSON []byte

	err := s.pool.QueryRow(context.Background(), query, name).Scan(
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

func (s *PostgresStore) LookupEntity(handle string) (*rdap.EntityRecord, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities WHERE handle = $1
	`

	var record rdap.EntityRecord
	var rolesJSON, statusJSON, pidJSON []byte

	err := s.pool.QueryRow(context.Background(), query, handle).Scan(
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

func (s *PostgresStore) LookupNameserver(name string) (*rdap.NameserverRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers WHERE ldh_name = $1 OR unicode_name = $1
	`

	var record rdap.NameserverRecord
	var ipv4JSON, ipv6JSON, statusJSON []byte

	err := s.pool.QueryRow(context.Background(), query, name).Scan(
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

func (s *PostgresStore) LookupIPNetwork(cidr string) (*rdap.IPNetworkRecord, error) {
	query := `
		SELECT handle, start_address, end_address, ip_version, cidr,
		       name, type, country, status, created_at, updated_at
		FROM ip_networks WHERE $1::inet <<= ANY(cidr)
	`

	var record rdap.IPNetworkRecord
	var cidrJSON, statusJSON []byte

	err := s.pool.QueryRow(context.Background(), query, cidr).Scan(
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

func (s *PostgresStore) SearchDomainsByName(pattern string, limit int) ([]rdap.DomainRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing
		FROM domains
		WHERE ldh_name LIKE $1 OR unicode_name LIKE $1
		LIMIT $2
	`

	sqlPattern := patternToSQL(pattern)
	rows, err := s.pool.Query(context.Background(), query, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search domains: %w", err)
	}
	defer rows.Close()

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

	return results, nil
}

func (s *PostgresStore) SearchDomainsByNS(nsName string, limit int) ([]rdap.DomainRecord, error) {
	query := `
		SELECT d.handle, d.ldh_name, d.unicode_name, d.tld, d.status,
		       d.created_at, d.updated_at, d.expires_at,
		       d.registrant, d.admin, d.tech, d.billing
		FROM domains d
		JOIN domain_nameservers dn ON d.handle = dn.domain_handle
		JOIN nameservers n ON dn.ns_handle = n.handle
		WHERE n.ldh_name = $1 OR n.unicode_name = $1
		LIMIT $2
	`

	rows, err := s.pool.Query(context.Background(), query, nsName, limit)
	if err != nil {
		return nil, fmt.Errorf("search domains by NS: %w", err)
	}
	defer rows.Close()

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

	return results, nil
}

func (s *PostgresStore) SearchEntitiesByName(pattern string, limit int) ([]rdap.EntityRecord, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities
		WHERE vcard_json ILIKE $1 OR handle ILIKE $1
		LIMIT $2
	`

	sqlPattern := "%" + pattern + "%"
	rows, err := s.pool.Query(context.Background(), query, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	defer rows.Close()

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

	return results, nil
}

func (s *PostgresStore) SearchEntitiesByHandle(pattern string, limit int) ([]rdap.EntityRecord, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities
		WHERE handle ILIKE $1
		LIMIT $2
	`

	sqlPattern := patternToSQL(pattern)
	rows, err := s.pool.Query(context.Background(), query, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search entities by handle: %w", err)
	}
	defer rows.Close()

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

	return results, nil
}

func (s *PostgresStore) SearchNameserversByName(pattern string, limit int) ([]rdap.NameserverRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers
		WHERE ldh_name LIKE $1 OR unicode_name LIKE $1
		LIMIT $2
	`

	sqlPattern := patternToSQL(pattern)
	rows, err := s.pool.Query(context.Background(), query, sqlPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search nameservers: %w", err)
	}
	defer rows.Close()

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

	return results, nil
}

func (s *PostgresStore) SearchNameserversByIP(ip string, limit int) ([]rdap.NameserverRecord, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers
		WHERE ipv4 @> $1::jsonb OR ipv6 @> $1::jsonb
		LIMIT $2
	`

	ipJSON := fmt.Sprintf(`["%s"]`, ip)
	rows, err := s.pool.Query(context.Background(), query, ipJSON, limit)
	if err != nil {
		return nil, fmt.Errorf("search nameservers by IP: %w", err)
	}
	defer rows.Close()

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

	return results, nil
}

func (s *PostgresStore) Ping() error {
	return s.pool.Ping(context.Background())
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

func patternToSQL(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "*", "%")
	pattern = strings.ReplaceAll(pattern, "?", "_")
	return pattern
}
