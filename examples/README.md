# Examples

Ready-to-run example databases and configurations for the RDAP server.

> **These are examples — not a production database.**
>
> Everything under `examples/` exists to (a) let you stand up the server in minutes
> for development, testing, or evaluation, and (b) document the exact table/column
> contract the server reads. The data is **fabricated sample data** (`example.com`
> registrations, fake contacts, placeholder IP/ASN allocations) that you must **not**
> serve to the public. It is **not** a full WHOIS/RDAP dataset: there is no bulk
> load tooling, no EPP/DROP feed integration, and no incremental updates.
>
> To serve real registration data you connect the server to your *own* database —
> see [Mapping to your existing database](#mapping-to-your-existing-database).

```
examples/
├── README.md            # this file
├── postgres/            # PostgreSQL schema + seed data (multiple example domains)
│   ├── schema.sql       # full schema (domains, entities, nameservers, ip_networks, autnums)
│   ├── seed.sql         # sample domains, entities, nameservers, networks, ASNs
│   └── config.yaml      # example config pointing at PostgreSQL
└── mysql/               # MySQL 8 schema + seed data
    ├── schema.sql       # full schema (MySQL dialect, JSON columns)
    ├── seed.sql         # sample domains, entities, nameservers, networks, ASNs
    └── config.yaml      # example config pointing at MySQL
```

## Mapping to your existing database

The RDAP server does **not** care which database you use — it only cares that the
database exposes the tables, columns, and data shapes documented below. There are
three ways to connect it to a real/existing database:

### Option 1 — your schema already matches (nothing to do)

If your database already has tables named `domains`, `entities`, `nameservers`,
`domain_nameservers`, `ip_networks`, and `autnums` with the columns listed in
[Schema contract](#schema-contract), just point `storage.dsn` at it. No code or SQL
changes are needed.

### Option 2 — different schema: adapt it with SQL views (recommended)

Most existing registration backends (EPP databases, WHOIS servers, SRS exports) use
different table and column names. You can **adapt your existing tables to the server's
expectations without touching the server code** by creating a set of views.

For example, say you already have an EPP-style `registrations` table:

```sql
CREATE TABLE registrations (
    roid          TEXT PRIMARY KEY,          -- e.g. 'EX1-NAME'
    punycode      TEXT NOT NULL,             -- ldh form
    unicode_name  TEXT NOT NULL,             -- unicode form
    tld           TEXT NOT NULL,
    epp_status    TEXT[] NOT NULL,           -- e.g. '{"associated"}'
    created_date  TIMESTAMPTZ NOT NULL,
    updated_date  TIMESTAMPTZ NOT NULL,
    expiry_date   TIMESTAMPTZ,
    registrant_id TEXT,
    tech_id       TEXT
);
```

Map it with a view that renames columns into the expected names and shapes:

```sql
CREATE VIEW domains AS
SELECT
    roid            AS handle,
    punycode        AS ldh_name,
    unicode_name,
    tld,
    -- server expects a JSON array of RFC 5731 status values
    epp_status      AS status,
    created_date    AS created_at,
    updated_date    AS updated_at,
    expiry_date     AS expires_at,
    registrant_id   AS registrant,
    NULL            AS admin,
    tech_id         AS tech,
    NULL            AS billing,
    -- server expects the embedded nameserver objects as JSON (see below)
    '[]'::jsonb     AS nameservers,
    NULL            AS secure_dns
FROM registrations;
```

Repeat for `entities`, `nameservers`, `ip_networks`, and `autnums`. Once the views
exist, set `storage.driver`/`storage.dsn` to your real database and start the server.
PostgreSQL and MySQL both support views, so this works for either backend.

### Option 3 — completely custom access path: implement a store

If your data lives in something views can't easily expose (a REST API, an LDAP
directory, a file, or a wildly different relational model), implement the storage
interface yourself. It is a small, stable seam:

```go
// internal/store/store.go
type Interface interface {
    LookupDomain(name string) (*rdap.DomainRecord, error)
    LookupEntity(handle string) (*rdap.EntityRecord, error)
    LookupNameserver(name string) (*rdap.NameserverRecord, error)
    LookupIPNetwork(cidr string) (*rdap.IPNetworkRecord, error)
    SearchDomainsByName(pattern string, limit int) ([]rdap.DomainRecord, error)
    SearchDomainsByNS(nsName string, limit int) ([]rdap.DomainRecord, error)
    SearchEntitiesByName(pattern string, limit int) ([]rdap.EntityRecord, error)
    SearchEntitiesByHandle(pattern string, limit int) ([]rdap.EntityRecord, error)
    SearchNameserversByName(pattern string, limit int) ([]rdap.NameserverRecord, error)
    SearchNameserversByIP(ip string, limit int) ([]rdap.NameserverRecord, error)
    Ping() error
    Close() error
}
```

Implement it against any backend, register it in `store.New()` (next to `memory`,
`postgres`, and `mysql`), and select it with `storage.driver`. The HTTP layer,
conformance, and response building are fully decoupled from storage.

### Schema contract

The built-in `postgres` and `mysql` stores read exactly these tables and columns.
This is the contract your existing database (or views) must satisfy:

| Table | Columns | Notes |
|-------|---------|-------|
| `domains` | `handle`, `ldh_name`, `unicode_name`, `tld`, `status`, `created_at`, `updated_at`, `expires_at`, `registrant`, `admin`, `tech`, `billing`, `nameservers`, `secure_dns` | `status` = JSON array of RFC 5731 statuses; `nameservers` = JSON array of nameserver objects (see below); `registrant`/`admin`/`tech`/`billing` = entity handles; `secure_dns` = JSON object or NULL |
| `entities` | `handle`, `vcard_json`, `roles`, `status`, `created_at`, `updated_at`, `public_ids` | `vcard_json` = serialized jCard (RFC 7095) as a JSON string; `roles`/`status` = JSON arrays; `public_ids` = JSON array of `{type, identifier}` |
| `nameservers` | `handle`, `ldh_name`, `unicode_name`, `ipv4`, `ipv6`, `status`, `created_at`, `updated_at` | `ipv4`/`ipv6` = JSON arrays of address strings |
| `domain_nameservers` | `domain_handle`, `ns_handle` | junction for `?nsLdhName=` searches |
| `ip_networks` | `handle`, `start_address`, `end_address`, `ip_version`, `cidr`, `name`, `type`, `country`, `status`, `created_at`, `updated_at` | PostgreSQL also needs `start_address`/`end_address` as `inet` + `cidr[]`; MySQL instead uses numeric `start_ip`/`end_ip` (`BIGINT UNSIGNED`, IPv4) and `start_ip6`/`end_ip6` (`VARBINARY(16)`, IPv6) range columns |
| `autnums` | `handle`, `start_asn`, `end_asn`, `name`, `type`, `country`, `status`, `created_at`, `updated_at` | |

Data-shape rules that matter for correctness:

- **Handles** must use the EPP ROID form `<local-id>-<repo-id>` (e.g. `EX1-NAME`).
  The part after the first hyphen is validated against the IANA
  [EPP Repository Identifiers](https://www.iana.org/assignments/epp-repository-ids/)
  registry by the ICANN conformance tool (`-46201`, `-47202`, `-63101`).
- **Status values** must come from RFC 5731 (e.g. `associated`, `active`,
  `clientTransferProhibited`) — not free text.
- **Both `ldh_name` and `unicode_name`** are required so IDN lookups and
  `unicodeName` output work (they are identical for ASCII names).
- **`vcard_json`** must be a valid jCard array string,
  `["vcard", [ ...properties... ]]`, not a JSON object or an HTML fragment.
- **`secure_dns`** shape (if present):
  `{"zoneSigned": bool, "delegationSigned": bool, "maxSigLife": int?,
    "dsData": [{"keyTag","algorithm","digestType","digest"}], "keyData": [...]}`.
- **`nameservers`** (embedded on `domains`) must be an array of objects like
  `{"handle","ldhName","unicodeName","ipv4":[],"ipv6":[],"status":[]}`.

If your existing database stores data in different shapes (e.g. status as a
comma-separated string, or addresses as integers), build that shape in the view with
`json_build_array`/`CAST` (PostgreSQL) or `JSON_ARRAY`/`CAST` (MySQL).

## PostgreSQL

```bash
# 1. Start PostgreSQL and load the schema + seed
psql -U rdap -d rdap -f examples/postgres/schema.sql
psql -U rdap -d rdap -f examples/postgres/seed.sql

# 2. Point the server at it
./rdapd -config examples/postgres/config.yaml

# 3. Try the seeded domains
curl http://localhost:8443/domain/example.com
curl http://localhost:8443/domain/example.net
curl http://localhost:8443/domain/example.org
curl http://localhost:8443/domain/xn--bcher-kva.com      # bücher.com (IDN)
curl http://localhost:8443/entity/2
curl http://localhost:8443/nameserver/ns1.example.com
```

### Seeded domains

| Domain | Handle | Registrant | Registrar | Nameservers |
|--------|--------|-----------|-----------|-------------|
| `example.com` | `EX1-NAME` | `REG1-NAME` | `2` | `NS1-NAME`, `NS2-NAME` |
| `example.net` | `EX2-NAME` | `REG2-NAME` | `2` | `NS3-NAME`, `NS4-NAME` |
| `example.org` | `EX3-NAME` | `REG3-NAME` | `2` | `NS1-NAME`, `NS3-NAME` |
| `bücher.com` (`xn--bcher-kva.com`) | `EX4-NAME` | `REG4-NAME` | `2` | `NS1-NAME`, `NS2-NAME` |
| `example.info` | `EX5-NAME` | `REG5-NAME` | `2` | `NS2-NAME`, `NS4-NAME` |

All handles use the `<local>-<EPPROID>` ROID form; the `NAME` suffix is a repository
ID registered in the [IANA EPP Repository Identifiers registry](https://www.iana.org/assignments/epp-repository-ids/).

## MySQL

MySQL 8.0+ is required (uses `JSON` columns and `JSON_CONTAINS`).

```bash
# 1. Start MySQL and load the schema + seed
mysql -u rdap -p < examples/mysql/schema.sql
mysql -u rdap -p < examples/mysql/seed.sql

# 2. Point the server at it
./rdapd -config examples/mysql/config.yaml

# 3. Try the same queries as PostgreSQL above
curl http://localhost:8443/domain/example.com
```

### How the server talks to each database

| Concern | PostgreSQL | MySQL |
|---------|-----------|-------|
| Driver | `jackc/pgx/v5` | `go-sql-driver/mysql` |
| Placeholders | `$1, $2, …` | `?, ?, …` |
| JSON columns | `jsonb` (`@>`, `ILIKE`) | `JSON` (`JSON_CONTAINS`, `LIKE`) |
| CIDR containment | `inet` + `cidr[]` (GIST) | numeric `BIGINT`/`VARBINARY(16)` range columns |
| Config `storage.driver` | `postgres` | `mysql` |

### Making it a registry

Switch the operator role in either config to serve registry-style responses:

```yaml
rdap:
  mode: "registry"
```

See the README's [Operator Modes](../README.md#operator-modes) section for details.