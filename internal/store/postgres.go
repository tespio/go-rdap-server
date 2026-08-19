package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/domain"
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

func (s *PostgresStore) LookupDomain(name string) (*domain.Domain, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
		FROM domains WHERE ldh_name = $1 OR unicode_name = $1
	`

	var record domain.Domain
	var statusJSON, nsJSON, sdJSON []byte
	var created, updated, expires time.Time
	var registrant, admin, tech, billing *string

	err := s.pool.QueryRow(context.Background(), query, name).Scan(
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
		Source:    "postgres",
	}

	record.Contacts = map[domain.ContactRole][]string{}
	if registrant != nil {
		record.Contacts[domain.RoleRegistrant] = []string{*registrant}
	}
	if admin != nil {
		record.Contacts[domain.RoleAdministrative] = []string{*admin}
	}
	if tech != nil {
		record.Contacts[domain.RoleTechnical] = []string{*tech}
	}
	if billing != nil {
		record.Contacts[domain.RoleBilling] = []string{*billing}
	}
	if registrant != nil {
		record.Registrar = *registrant
	}

	// The example schema stores full nameserver objects in a JSON column.
	if len(nsJSON) > 0 {
		record.Nameservers = parseNameservers(nsJSON)
	}

	return &record, nil
}

func (s *PostgresStore) LookupContact(handle string) (*domain.Contact, error) {
	query := `
		SELECT handle, vcard_json, roles, status, created_at, updated_at, public_ids
		FROM entities WHERE handle = $1
	`

	var record domain.Contact
	var rolesJSON, statusJSON, pidJSON []byte
	var vcardJSON *string
	var created, updated time.Time

	err := s.pool.QueryRow(context.Background(), query, handle).Scan(
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
		Source:    "postgres",
	}

	return &record, nil
}

func (s *PostgresStore) LookupNameserver(name string) (*domain.NameServer, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, ipv4, ipv6, status, created_at, updated_at
		FROM nameservers WHERE ldh_name = $1 OR unicode_name = $1
	`

	var record domain.NameServer
	var ipv4JSON, ipv6JSON, statusJSON []byte
	var created, updated time.Time

	err := s.pool.QueryRow(context.Background(), query, name).Scan(
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
		Source:    "postgres",
	}

	return &record, nil
}

func (s *PostgresStore) LookupIPNetwork(cidr string) (*domain.IPNetwork, error) {
	query := `
		SELECT handle, start_address, end_address, ip_version, cidr,
		       name, type, country, status, created_at, updated_at
		FROM ip_networks WHERE $1::inet <<= ANY(cidr)
	`

	var record domain.IPNetwork
	var cidrJSON, statusJSON []byte
	var created, updated time.Time

	err := s.pool.QueryRow(context.Background(), query, cidr).Scan(
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
		Source:    "postgres",
	}

	return &record, nil
}

func (s *PostgresStore) LookupAutnum(asn int) (*domain.Autnum, error) {
	query := `
		SELECT handle, start_asn, end_asn, name, type, country, status, created_at, updated_at
		FROM autnums WHERE start_asn <= $1 AND end_asn >= $1
		LIMIT 1
	`

	var record domain.Autnum
	var statusJSON []byte
	var created, updated time.Time

	err := s.pool.QueryRow(context.Background(), query, asn).Scan(
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
		Source:    "postgres",
	}

	return &record, nil
}

func (s *PostgresStore) SearchDomainsByName(pattern string, limit int) ([]domain.Domain, error) {
	query := `
		SELECT handle, ldh_name, unicode_name, tld, status,
		       created_at, updated_at, expires_at,
		       registrant, admin, tech, billing,
		       nameservers, secure_dns
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

	var results []domain.Domain
	for rows.Next() {
		d, err := scanDomainRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *d)
	}
	return results, rows.Err()
}

func (s *PostgresStore) SearchDomainsByNS(nsName string, limit int) ([]domain.Domain, error) {
	query := `
		SELECT d.handle, d.ldh_name, d.unicode_name, d.tld, d.status,
		       d.created_at, d.updated_at, d.expires_at,
		       d.registrant, d.admin, d.tech, d.billing,
		       d.nameservers, d.secure_dns
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

	var results []domain.Domain
	for rows.Next() {
		d, err := scanDomainRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *d)
	}
	return results, rows.Err()
}

// domainRowScanner is implemented by both *pgx.Rows and *pgx.Row via Scan.
type domainRowScanner interface {
	Scan(dest ...any) error
}

func scanDomainRow(row domainRowScanner) (*domain.Domain, error) {
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
		Source:    "postgres",
	}

	record.Contacts = map[domain.ContactRole][]string{}
	if registrant != nil {
		record.Contacts[domain.RoleRegistrant] = []string{*registrant}
		record.Registrar = *registrant
	}
	if admin != nil {
		record.Contacts[domain.RoleAdministrative] = []string{*admin}
	}
	if tech != nil {
		record.Contacts[domain.RoleTechnical] = []string{*tech}
	}
	if billing != nil {
		record.Contacts[domain.RoleBilling] = []string{*billing}
	}

	if len(nsJSON) > 0 {
		record.Nameservers = parseNameservers(nsJSON)
	}

	return &record, nil
}

func (s *PostgresStore) SearchContactsByName(pattern string, limit int) ([]domain.Contact, error) {
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

	var results []domain.Contact
	for rows.Next() {
		c, err := scanContactRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *c)
	}
	return results, rows.Err()
}

func (s *PostgresStore) SearchContactsByHandle(pattern string, limit int) ([]domain.Contact, error) {
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

	var results []domain.Contact
	for rows.Next() {
		c, err := scanContactRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *c)
	}
	return results, rows.Err()
}

func scanContactRow(row domainRowScanner) (*domain.Contact, error) {
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
		Source:    "postgres",
	}

	return &record, nil
}

func (s *PostgresStore) SearchNameserversByName(pattern string, limit int) ([]domain.NameServer, error) {
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

	var results []domain.NameServer
	for rows.Next() {
		n, err := scanNameserverRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *n)
	}
	return results, rows.Err()
}

func (s *PostgresStore) SearchNameserversByIP(ip string, limit int) ([]domain.NameServer, error) {
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

	var results []domain.NameServer
	for rows.Next() {
		n, err := scanNameserverRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *n)
	}
	return results, rows.Err()
}

func scanNameserverRow(row domainRowScanner) (*domain.NameServer, error) {
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
		Source:    "postgres",
	}

	return &record, nil
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
