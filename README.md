# RDAP Server

[![Go version](https://img.shields.io/badge/Go-1.22+-blue?logo=go&logoColor=white)](https://go.dev/dl)
[![CI](https://img.shields.io/github/actions/workflow/status/tespio/go-rdap-server/ci.yml?branch=master&label=CI&logo=github)](https://github.com/tespio/go-rdap-server/actions)
[![ICANN Conformance](https://img.shields.io/github/actions/workflow/status/tespio/go-rdap-server/conformance.yml?branch=master&label=ICANN%20RDAPCT%20CI&logo=github)](https://github.com/tespio/go-rdap-server/actions/workflows/conformance.yml)
[![Release](https://img.shields.io/github/v/release/tespio/go-rdap-server?logo=github)](https://github.com/tespio/go-rdap-server/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/Coverage-6.7%25-critical?logo=codecov&logoColor=white)](#testing--coverage)
[![RDAPCT: 2024 Registrar - 0 errors](https://img.shields.io/badge/RDAPCT-2024%20Registrar%20%E2%9C%94%200%20errors-brightgreen)](README.md#icann-conformance)
[![RDAPCT: 2024 Registry - 0 errors*](https://img.shields.io/badge/RDAPCT-2024%20Registry%20%E2%9C%94%200%20errors*%20-blue)](README.md#icann-conformance)

A production-ready [Registration Data Access Protocol (RDAP)](https://datatracker.ietf.org/doc/html/rfc9082)
server built in Go. It serves registration data (domains, entities, nameservers, IP
networks, and autonomous system numbers) over HTTP/HTTPS as JSON, and is designed to
operate as either a **gTLD registry** or a **gTLD registrar** RDAP service.

> ### ✅ ICANN RDAP Conformance — verified with the official tool
>
> Tested against **rdapct v3.1.0** (the [ICANN RDAP Conformance Tool](https://icann.github.io/rdap-conformance-tool/))
> over HTTPS: **STD 95 — 31 groups / 0 errors**, **gTLD Registrar 2019 profile — 59 / 0**,
> **gTLD Registrar 2024 profile — 78 / 0**, **gTLD Registry 2019 & 2024 — 0 errors**
> apart from the `-23101` IANA-bootstrap registration constraint (`*`).
>
> Full matrix, prerequisites, and the exact commands to reproduce it are in the
> [ICANN Conformance](#icann-conformance) section.

## Why this exists

Most off-the-shelf RDAP stacks are heavyweight Java applications that are hard to
operate, tune, and keep current with the [gTLD RDAP Profile (Feb 2024)](https://www.icann.org/en/contracted-parties/registry-operators/registration-data-access-protocol/gtld-rdap-profile-01-01-2020-en)
and its [RFC 9537 redaction rules](https://www.rfc-editor.org/rfc/rfc9537). This project
is a small, single-binary Go alternative: no JVM, no application server, fast startup,
static binaries for every major OS/arch, and a tiny memory footprint — while still
passing the official conformance suite for both registrar and registry operator modes.
It was built for operators who need a compliant, embeddable, or container-friendly RDAP
service without the operational overhead.

## Features

- **RFC 9082/9083 (STD 95) compliant** — lookups and searches for domains, entities,
  nameservers, IP networks, and autnums.
- **2019 and 2024 gTLD RDAP Profiles** — ICANN Response Profile + Technical
  Implementation Guide (single conformance array covers both).
- **Dual operator modes** — `registrar` or `registry` (see
  [Operator Modes](#operator-modes)).
- **IDN support** — Internationalized Domain Names via IDNA 2008
  (`unicodeName`/`ldhName`).
- **DNSSEC** — `secureDNS` with DS records and `delegationSigned`.
- **jCard vCards** — RFC 6350/7095 JSON vCard contact data for entities.
- **Dual storage backends** — in-memory (development), PostgreSQL, and MySQL (production).
- **Rate limiting** — per-IP, configurable window and burst, with trusted-proxy client-IP resolution (spoof-proof).
- **Optional authentication** — JWT/Bearer token via JWKS.
- **Prometheus metrics** — built-in `/metrics`-style endpoint.
- **CORS + security headers** — ready for browser clients.
- **HTTPS/TLS** — native TLS termination or reverse-proxy termination
  (`X-Forwarded-Proto`).
- **Docker** — multi-stage image plus a full `docker-compose` stack.

## Table of Contents

- [Quick Start](#quick-start)
- [Operator Modes](#operator-modes)
- [Configuration](#configuration)
- [API Endpoints](#api-endpoints)
- [Example Responses](#example-responses)
- [Storage](#storage)
- [Examples](#examples)
- [ICANN Conformance](#icann-conformance)
- [Production Deployment](#production-deployment)
- [RFC 9537 Redaction (2024 Profile)](#rfc-9537-redaction-2024-profile)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Testing & Coverage](#testing--coverage)
- [Development](#development)
- [License](#license)

## Quick Start

Requirements: [Go 1.22+](https://go.dev/dl).

```bash
# Build
go build -o rdapd.exe ./cmd/rdapd

# Run with the in-memory store (no external dependencies)
./rdapd.exe -config config.yaml

# Or with the Makefile
make run
```

The server listens on `:8443` (RDAP) and `:9090` (metrics) by default.

```bash
# Smoke test
curl -i http://localhost:8443/help
curl -i http://localhost:8443/domain/example.com
curl -i http://localhost:8443/nameserver/ns1.example.com
curl -i http://localhost:8443/entity/2
curl -i http://localhost:8443/ip/8.8.8.0/24
curl -i http://localhost:8443/autnum/15169
curl -i http://localhost:8443/domain/not-a-domain.invalid   # → 404 (RFC 7482 §4.1)
```

## Operator Modes

Set `rdap.mode` in `config.yaml` to tell the server whether it is operated by a
registrar or a registry. This controls which data the domain responses include.

| Mode | Registrant entity | `registrar expiration` event | Typical operator |
|------|:---:|:---:|------------------|
| `registrar` (default) | ✅ included | ✅ included | Registrars (have full contact data) |
| `registry` | ❌ omitted | ❌ omitted | Registries (thin/thick) |

```yaml
rdap:
  mode: "registrar"   # or "registry"
```

Rationale:

- **Registrar servers** must return a `registrant` role entity (`-63000`) and a
  `registrar expiration` event (`-65600`) per the 2024 gTLD Response Profile.
- **Registry servers** usually do not publish registrant contact data; omitting the
  entity is valid, and the registrant/technical tests only apply *if present*.
- **Registry servers** additionally require the queried TLD's RDAP base URL to be
  registered in the [IANA DNS RDAP bootstrap file](https://www.iana.org/domains/rdap)
  (`-23101`); see [ICANN Conformance](#icann-conformance).

## Configuration

The server reads a YAML file (`-config config.yaml`). Every field is optional except
`rdap.base_url`; sensible defaults are applied.

```yaml
server:
  host: "0.0.0.0"            # bind address ("" or "0.0.0.0" = all interfaces, dual-stack)
  port: 8443                 # RDAP listener port
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 60s
  max_header_bytes: 1048576
  tls_cert_file: "/etc/rdap/certs/tls.crt"   # set both to enable TLS
  tls_key_file:  "/etc/rdap/certs/tls.key"

storage:
  driver: "memory"           # "memory" | "postgres"
  dsn: "postgres://rdap:rdap@localhost:5432/rdap?sslmode=disable"
  max_open_conns: 25
  max_idle_conns: 5
  cache_ttl: "5m"

rdap:
  mode: "registrar"          # "registrar" | "registry"
  tlds: ["com", "net", "org", "io", "ai", ...]
  base_url: "https://rdap.example.com"       # your public RDAP base URL (required)
  registrar_base_url: "https://rdap.example.org/rdap/"  # example registrar RDAP base URL
  max_domain_length: 253
  max_search_limit: 100
  port43_whois: "whois.example.com"
  server_name: "RDAP Server v1.2"
  version: "1.2.0"
  # Customize the Terms of Service notice (card + link) in responses.
  # Set the URL to your real terms page, like a registrar's agreement.
  tos:
    title: "Terms of Service"
    description:
      - "Registration data for example.com is provided by Example Registrar, Inc."
    url: "https://rdap.example.com/help"
  # Optional registrar/registry-specific notices appended to every response.
  # The ICANN-mandated notices (Status Codes, RDDS Inaccuracy) are always included.
  custom_notices:
    - title: "Data Policy"
      description:
        - "Contact data is published per the ICANN Registration Data Policy."
      url: "https://rdap.example.com/privacy"
      rel: "privacy-policy"

auth:
  enabled: false
  jwks_endpoint: "https://auth.example.com/.well-known/jwks.json"
  issuer: "https://auth.example.com"
  audience: "rdap.example.com"

metrics:
  enabled: true
  host: "0.0.0.0"
  port: 9090

rate_limiting:
  enabled: true
  requests: 100             # per IP per window
  window: 1m
  burst: 50
  # trusted_ips = the ONLY sources allowed to set X-Forwarded-For / X-Real-IP.
  # Requests from any other peer are rate-limited by their real socket IP and
  # their forwarded headers are ignored. Set to your proxy/LB addresses only.
  trusted_ips: ["127.0.0.1", "10.0.0.0/8", "192.168.0.0/16"]
```

| Key | Default | Description |
|-----|---------|-------------|
| `server.host` | `0.0.0.0` | Bind address; `""`/`0.0.0.0` binds all interfaces (IPv4+IPv6) |
| `server.port` | `8443` | RDAP listener port |
| `server.tls_cert_file` / `server.tls_key_file` | *(unset)* | Enable TLS when both set |
| `storage.driver` | `memory` | `memory`, `postgres`, or `mysql` |
| `storage.dsn` | *(unset)* | PostgreSQL connection string (required for `postgres`) |
| `rdap.mode` | `registrar` | `registrar` or `registry` |
| `rdap.tlds` | *(unset)* | Allowed TLDs for domain lookups |
| `rdap.base_url` | *(required)* | Public base URL; used for self links and notice hrefs |
| `rdap.registrar_base_url` | `base_url` | RDAP base URL of the registrar (used for the `about`/`related` links). The shipped config uses the example URL `https://rdap.example.org/rdap/`; for ICANN conformance (`-47701`) set it to the base URL registered in the IANA registrar-ids dataset for the registrar IANA ID in your data (e.g. ID 2 → `https://rdap.networksolutions.com/rdap/`) |
| `rdap.max_domain_length` | `253` | Max domain name length |
| `rdap.max_search_limit` | `100` | Max results for search endpoints |
| `rdap.port43_whois` | *(unset)* | Whois server host for the `port43` member |
| `rdap.server_name` | *(unset)* | Server display name |
| `rdap.version` | `1.2` | Server version string |
| `rdap.tos.title` | `Terms of Service` | Title of the Terms of Service notice card |
| `rdap.tos.description` | *(generic)* | Body text of the ToS notice (company name, terms) |
| `rdap.tos.url` | `{base_url}/help` | ToS link target (`rel: terms-of-service`) |
| `rdap.custom_notices` | *(none)* | List of `{title, description, url, rel}` registrar-specific notices appended to responses |
| `auth.enabled` | `false` | Enable JWT authentication |
| `metrics.enabled` | `true` | Enable the Prometheus metrics endpoint |
| `rate_limiting.enabled` | `true` | Enable per-IP rate limiting |
| `rate_limiting.trusted_ips` | *(empty)* | Addresses/CIDRs allowed to set `X-Forwarded-For`/`X-Real-IP` (your proxies). All other peers are limited by their real socket IP. |

## API Endpoints

### Lookups

| Method | Path | Description |
|--------|------|-------------|
| `GET`/`HEAD` | `/help` | Help, supported endpoints, and notices |
| `GET`/`HEAD` | `/domain/{name}` | Domain lookup |
| `GET`/`HEAD` | `/entity/{handle}` | Entity lookup (registrar/registrant/contact) |
| `GET`/`HEAD` | `/nameserver/{name}` | Nameserver lookup |
| `GET`/`HEAD` | `/ip/{network}` | IP network lookup (`CIDR`) |
| `GET`/`HEAD` | `/autnum/{asn}` | Autonomous system number lookup |

`HEAD` returns the same status code and headers as `GET` (required by TIG 1.6).

### Searches (RFC 7482)

| Method | Path | Description |
|--------|------|-------------|
| `GET`/`HEAD` | `/domains?name={pattern}` | Search domains by name (wildcards `*` allowed) |
| `GET`/`HEAD` | `/domains?nsLdhName={ns}` | Search domains by nameserver |
| `GET`/`HEAD` | `/entities?fn={pattern}` | Search entities by full name |
| `GET`/`HEAD` | `/entities?handle={pattern}` | Search entities by handle |
| `GET`/`HEAD` | `/nameservers?name={pattern}` | Search nameservers by name |
| `GET`/`HEAD` | `/nameservers?ip={address}` | Search nameservers by IP |

All search responses are arrays of the matched objects. Add `?limit=n` to cap results
(hard cap = `rdap.max_search_limit`).

### Examples

```bash
# Lookups
curl "http://localhost:8443/domain/example.com"
curl "http://localhost:8443/entity/2"
curl "http://localhost:8443/nameserver/ns1.example.com"
curl "http://localhost:8443/ip/8.8.8.0/24"
curl "http://localhost:8443/autnum/15169"

# Searches
curl "http://localhost:8443/domains?name=example*"
curl "http://localhost:8443/domains?nsLdhName=ns1.example.com"
curl "http://localhost:8443/entities?fn=Example*"
curl "http://localhost:8443/entities?handle=REG1*"
curl "http://localhost:8443/nameservers?name=ns1*"
curl "http://localhost:8443/nameservers?ip=8.8.8.8"
```

## Example Responses

### Domain lookup

`GET /domain/example.com` (registrar mode)

```json
{
  "objectClassName": "domain",
  "handle": "EX1-NAME",
  "ldhName": "example.com",
  "unicodeName": "example.com",
  "status": ["active"],
  "events": [
    {"eventAction": "registration", "eventDate": "2025-08-18T20:00:00Z"},
    {"eventAction": "last changed", "eventDate": "2026-08-18T20:00:00Z"},
    {"eventAction": "expiration", "eventDate": "2027-08-18T20:00:00Z"},
    {"eventAction": "last update of RDAP database", "eventDate": "2026-08-18T20:00:00Z"},
    {"eventAction": "registrar expiration", "eventDate": "2027-08-18T20:00:00Z"}
  ],
  "nameservers": [
    {
      "objectClassName": "nameserver",
      "handle": "NS1-NAME",
      "ldhName": "ns1.example.com",
      "ipAddresses": {"v4": ["8.8.8.8"], "v6": ["2001:4860:4860::8888"]},
      "links": [{
        "rel": "self",
        "href": "https://rdap.example.com/nameserver/ns1.example.com",
        "type": "application/rdap+json",
        "value": "https://rdap.example.com/domain/example.com"
      }]
    }
  ],
  "entities": [
    {
      "objectClassName": "entity",
      "handle": "2",
      "roles": ["registrar"],
      "publicIds": [{"type": "IANA Registrar ID", "identifier": "2"}],
      "vcardArray": [
        "vcard",
        [
          ["version", {}, "text", "4.0"],
          ["fn", {}, "text", "Example Registrar Inc."],
          ["adr", {"cc": "US"}, "text", ["", "", "123 Maple Ave", "Los Angeles", "CA", "90210", ""]]
        ]
      ],
      "links": [{
        "rel": "about",
        "href": "https://rdap.example.org/rdap/",
        "type": "application/rdap+json",
        "value": "https://rdap.example.org/rdap/"
      }],
      "entities": [{
        "objectClassName": "entity",
        "handle": "ABUSE-NAME",
        "roles": ["abuse"],
        "vcardArray": [
          "vcard",
          [
            ["version", {}, "text", "4.0"],
            ["fn", {}, "text", "Abuse Contact"],
            ["tel", {"type": ["voice"]}, "uri", "tel:+1-555-123-4567"],
            ["email", {}, "text", "abuse@example.com"]
          ]
        ]
      }]
    },
    {
      "objectClassName": "entity",
      "handle": "REG1-NAME",
      "roles": ["registrant"],
      "vcardArray": [
        "vcard",
        [
          ["version", {}, "text", "4.0"],
          ["fn", {}, "text", "Example Registrant"],
          ["org", {}, "text", "Example Organization"],
          ["adr", {"cc": "US"}, "text", ["", "", "123 Elm Street", "Springfield", "IL", "62701", ""]],
          ["tel", {"type": ["voice"]}, "uri", "tel:+1-217-555-0132"],
          ["email", {}, "text", "registrant@example.com"]
        ]
      ]
    }
  ],
  "secureDNS": {"zoneSigned": false, "delegationSigned": false},
  "links": [
    {"rel": "self", "href": "https://rdap.example.com/domain/example.com", "type": "application/rdap+json", "value": "https://rdap.example.com/domain/example.com"},
    {"rel": "related", "href": "https://rdap.example.org/rdap/domain/example.com", "type": "application/rdap+json", "value": "https://rdap.example.com/domain/example.com"}
  ],
  "rdapConformance": [
    "rdap_level_0",
    "icann_rdap_technical_implementation_guide_0",
    "icann_rdap_response_profile_0",
    "icann_rdap_technical_implementation_guide_1",
    "icann_rdap_response_profile_1"
  ],
  "notices": [
    {
      "title": "Status Codes",
      "description": ["For more information on domain status codes, please visit https://icann.org/epp"],
      "links": [{"rel": "glossary", "href": "https://icann.org/epp", "type": "text/html", "value": "<request url>"}]
    },
    {
      "title": "RDDS Inaccuracy Complaint Form",
      "description": ["URL of the ICANN RDDS Inaccuracy Complaint Form: https://icann.org/wicf"],
      "links": [{"rel": "help", "href": "https://icann.org/wicf", "type": "text/html", "value": "<request url>"}]
    }
  ]
}
```

> **Note:** the `value` member of every link reflects the *actual* request URL
> (scheme auto-detected from `X-Forwarded-Proto`/TLS, port added when the `Host`
> header omits it).

### Error response

Not-found objects return `404` with an RDAP error object (RFC 9083 §6):

```json
{
  "errorCode": 404,
  "title": "Domain not found",
  "description": ["No domain found for: example.invalid"],
  "lang": "en"
}
```

## Storage

The server supports three storage backends, selected with `storage.driver`:
`memory` (development), `postgres`, or `mysql` (production).

| Capability | memory | PostgreSQL | MySQL |
|-----------|:---:|:---:|:---:|
| Zero-config, seeded sample data | ✅ | – | – |
| JSON columns | – | `jsonb` | `JSON` |
| CIDR lookup | native | `inet`/`cidr[]` (GIST) | numeric range columns (`BIGINT`/`VARBINARY(16)`) |
| Connection pooling | – | `pgxpool` | `database/sql` |
| Migration file | – | `migrations/001_init.sql` | `migrations/002_mysql_init.sql` |

### In-memory (development)

The default `memory` store is seeded with sample data:

| Type | Handle / key | Notes |
|------|--------------|-------|
| Domain | `EX1-NAME` (`example.com`) | status `["active"]`, registrar `2`, tech `888` |
| Registrar entity | `2` | IANA Registrar ID 2 (Network Solutions sample) |
| Technical entity | `888` | |
| Nameservers | `NS1-NAME`, `NS2-NAME` | `ns1.example.com`, `ns2.example.com` |
| IP network | `8.8.8.0/24` | |
| Autnum | `15169` | |

The sample handles follow the EPP ROID shape `<local-id>-<EPPROID>` where the suffix
is a repository ID registered in the IANA [EPP Repository Identifiers registry](https://www.iana.org/assignments/epp-repository-ids/).

### PostgreSQL (production)

```yaml
storage:
  driver: "postgres"
  dsn: "postgres://user:pass@localhost:5432/rdap?sslmode=require"
```

Apply the schema (domains, entities, nameservers, domain_nameservers, ip_networks,
autnums, audit_log):

```bash
make migrate-up          # go run ./cmd/migrate -direction up
# or
psql -U rdap -d rdap -f migrations/001_init.sql
```

Implementation: `internal/store/postgres.go` (uses `jackc/pgx/v5`).

### MySQL (production)

MySQL 8.0+ is required. The DSN can use the native
[go-sql-driver](https://github.com/go-sql-driver/mysql) format or a `mysql://` URL
(accepted for consistency with the `postgres://` form):

```yaml
storage:
  driver: "mysql"
  dsn: "mysql://rdap:rdap@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4"
```

Apply the schema:

```bash
mysql -u rdap -p < migrations/002_mysql_init.sql
```

Because MySQL has no native CIDR/inet type, IP networks are stored as numeric ranges
(`start_ip`/`end_ip` for IPv4 as `BIGINT UNSIGNED`, `start_ip6`/`end_ip6` for IPv6 as
`VARBINARY(16)`); the server computes the queried range and matches numerically.

Implementation: `internal/store/mysql.go` (uses `database/sql` + `go-sql-driver/mysql`).

## Examples

Ready-to-run example databases (five domains, entities, nameservers, networks and
ASNs) with per-database configs live in the [`examples/`](examples/) directory:

| Example | Schema | Seed | Config |
|---------|--------|------|--------|
| PostgreSQL | `examples/postgres/schema.sql` | `examples/postgres/seed.sql` | `examples/postgres/config.yaml` |
| MySQL | `examples/mysql/schema.sql` | `examples/mysql/seed.sql` | `examples/mysql/config.yaml` |

```bash
# PostgreSQL
psql -U rdap -d rdap -f examples/postgres/schema.sql
psql -U rdap -d rdap -f examples/postgres/seed.sql
./rdapd -config examples/postgres/config.yaml

# MySQL
mysql -u rdap -p < examples/mysql/schema.sql
mysql -u rdap -p < examples/mysql/seed.sql
./rdapd -config examples/mysql/config.yaml
```

Seeded domains include `example.com`, `example.net`, `example.org`, `example.info` and
the IDN `bücher.com` (`xn--bcher-kva.com`).

> **Note:** these databases contain fabricated sample data for development and
> documentation only. For production you connect the server to your *own* database.
> The `examples/` directory also documents the exact schema contract the server reads
> and three ways to map it to an existing database (direct match, SQL views, or a
> custom store). See [`examples/README.md`](examples/README.md#mapping-to-your-existing-database).

## ICANN Conformance

The server is validated against the official
[ICANN RDAP Conformance Tool](https://icann.github.io/rdap-conformance-tool/)
(v3.1.0, `rdapct-3.1.0.jar`). Results (queried over HTTPS):

| Test configuration | Groups | Errors |
|--------------------|-------:|-------:|
| STD 95 (RFC 9082/9083) | 31 | 0 |
| gTLD Registrar — 2019 profile | 59 | 0 |
| gTLD Registrar — 2024 profile | 78 | 0 |
| gTLD Registry — 2019 profile | 60 | 2* |
| gTLD Registry — 2024 profile | 78 | 2* |

> **Automatically verified in CI.** The `Conformance` workflow
> (`.github/workflows/conformance.yml`) rebuilds the server, starts it with TLS, runs
> the official rdapct against it (gTLD Registrar, **2024 profile**), and **fails the
> build if conformance regresses**. Every push/PR confirms **78 groups / 0 errors**
> before the "we pass ICANN conformance" claim is allowed to stand. See the
> [Conformance workflow](https://github.com/tespio/go-rdap-server/actions/workflows/conformance.yml).

\* Only `-23101`: the queried TLD's RDAP base URL must be registered in the
[IANA DNS RDAP bootstrap](https://www.iana.org/domains/rdap). A production registry
must register its real base URL in the bootstrap file (e.g. `.com` points to Verisign).
This is a registration/data constraint, not a server defect.

> **About `registrar_base_url` and `-47701`:** the shipped configs use the placeholder
> URL `https://rdap.example.org/rdap/` because it is an *example*. The conformance
> test `-47701` requires the `about` link of the registrar entity to equal the base URL
> registered for that registrar's IANA ID in the [registrar-ids dataset](https://www.iana.org/assignments/registrar-ids/).
> The conformance runs above were executed with `rdap.registrar_base_url` set to the
> registered value (`https://rdap.networksolutions.com/rdap/` for IANA Registrar ID 2).
> Before running the tool, set it to the registered URL for the registrar IANA ID in
> your data.

### Prerequisites for local conformance runs

The gTLD profiles require HTTPS (TIG 1.2). rdapct runs on **Java 21+** and needs the
IANA datasets (downloads them on first run).

1. Install the tool:
   ```bash
   # Download from https://github.com/icann/rdap-conformance-tool/releases
   java -jar rdapct-3.1.0.jar --help
   ```

2. Generate a self-signed certificate and a combined truststore:
   ```bash
   # cert for 127.0.0.1.nip.io (a wildcard DNS name that resolves to 127.0.0.1,
   # so notice-link hosts pass URL validation). Put tls.crt/tls.key where your
   # config points, then build a truststore that ALSO contains the public CAs:
   #   cp $JAVA_HOME/lib/security/cacerts combined.jks
   #   keytool -importcert -alias rdap -file tls.crt -keystore combined.jks -storepass changeit
   ```

3. Run the server with TLS enabled (`tls_cert_file`/`tls_key_file` set), then:

```bash
# STD 95 (RFC compliance)
java -Djavax.net.ssl.trustStore=combined.jks -Djavax.net.ssl.trustStorePassword=changeit \
  -jar rdapct-3.1.0.jar -c rdapct_config.json \
  --no-ipv6-queries --additional-conformance-queries \
  https://127.0.0.1.nip.io:8443/domain/example.com

# gTLD Registrar — 2024 profile (use --use-rdap-profile-february-2019 for 2019)
java -Djavax.net.ssl.trustStore=combined.jks -Djavax.net.ssl.trustStorePassword=changeit \
  -jar rdapct-3.1.0.jar -c rdapct_config.json \
  --gtld-registrar --use-rdap-profile-february-2024 \
  --no-ipv6-queries --additional-conformance-queries \
  https://127.0.0.1.nip.io:8443/domain/example.com

# gTLD Registry — 2024 profile (run the server with rdap.mode: "registry")
java -Djavax.net.ssl.trustStore=combined.jks -Djavax.net.ssl.trustStorePassword=changeit \
  -jar rdapct-3.1.0.jar -c rdapct_config.json \
  --gtld-registry --use-rdap-profile-february-2024 \
  --no-ipv6-queries --additional-conformance-queries \
  https://127.0.0.1.nip.io:8443/domain/example.com
```

Notes:

- `--no-ipv6-queries` avoids connection failures on hosts without IPv6.
- `--additional-conformance-queries` additionally tests `/help` and
  `/domain/not-a-domain.invalid`.
- Results are written to `results/results-<timestamp>.json`.

### Docker method

```bash
docker buildx build -t rdapct .
docker run rdapct --gtld-registrar --use-rdap-profile-february-2024 \
  https://rdap.example.com/domain/example.com
```

### Conformance matrix (key tests)

| Test | Requirement | Status |
|------|-------------|--------|
| `-10505` | `rdapConformance` NOT in embedded objects | ✓ |
| `-13000` | Content-Type `application/rdap+json` | ✓ |
| `-13006` | `test.invalid` returns 404 | ✓ |
| `-46200` | Handle format `(\w\|_){1,80}-\w{1,8}` | ✓ |
| `-46600` | Status Codes notice → `https://icann.org/epp` | ✓ |
| `-46700` | RDDS Inaccuracy notice → `https://icann.org/wicf` | ✓ |
| `-46801` | `delegationSigned` in `secureDNS` | ✓ |
| `-46900` | Status values comply with RFC 5731 | ✓ |
| `-47300` | Registrar entity in domain | ✓ |
| `-47500` | Abuse entity with tel + email | ✓ |
| `-47700` | Registrar entity has `rel="about"` link | ✓ |
| `-47701` | About link matches IANA-registered registrar base URL | ✓ |
| `-63000` | Registrar server returns a registrant entity | ✓ |
| `-65600` | `registrar expiration` event (registrar mode) | ✓ |
| `-20300` | HEAD status equals GET status | ✓ |
| `-20100` | HTTPS only (TIG 1.2) | ✓ |

## Production Deployment

### HTTPS

- Serve over HTTPS only (TIG 1.2). Either terminate TLS natively
  (`server.tls_cert_file`/`tls_key_file`) or at a reverse proxy and forward
  `X-Forwarded-Proto: https`.
- If an HTTP listener is exposed, redirect to HTTPS.
- The link `value` fields auto-detect the scheme, so they always equal the request URL.

### Reverse proxy

Example nginx snippet (terminate TLS, forward headers):

```nginx
server {
    listen 443 ssl;
    server_name rdap.example.com;
    ssl_certificate     /etc/letsencrypt/live/rdap.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/rdap.example.com/privkey.pem;

    location / {
        proxy_pass         http://127.0.0.1:8443;
        proxy_set_header   Host $host;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_set_header   X-Forwarded-For $remote_addr;
    }
}
```

> **Important:** because this forwards `X-Forwarded-For`, the nginx host's address
> must be in `rate_limiting.trusted_ips` (see [Rate limiting](#rate-limiting)).
> If you proxy directly with `127.0.0.1`, add `127.0.0.1`; if nginx runs on a
> dedicated host/LB, add that host's IP or subnet. Only trusted proxies may set
> forwarded headers.

### Docker

```bash
# Build the image
docker build -t rdap-server .

# Run standalone (in-memory store)
docker run -p 8443:8443 -p 9090:9090 rdap-server

# Full stack: RDAP server + PostgreSQL + Prometheus + Grafana
docker-compose up -d
```

### Monitoring

Prometheus metrics are exposed on the metrics listener (`:9090` by default) at
`/metrics`. A scrape config is provided in `prometheus.yml`.

### Rate limiting

Per-IP rate limiting (`rate_limiting.requests` per `rate_limiting.window`) with a
burst allowance. Rate-limit headers are exposed:
`X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`.

**Client IP resolution behind a proxy (important):** the server only honors
`X-Forwarded-For` / `X-Real-IP` when the *direct network peer* is listed in
`rate_limiting.trusted_ips`. For any other peer those headers are ignored and the
real socket address is used for both rate limiting and logging. This prevents an
Internet client from spoofing its IP via headers to bypass per-IP limits.

- Put **only** your reverse proxy / load balancer addresses in `trusted_ips`.
- Do **not** include public/internet ranges (e.g. `0.0.0.0/0`) — that would let
  anyone spoof forwarded headers.
- When running without a proxy, you can leave `trusted_ips` empty (or only
  `127.0.0.1`); all forwarded headers are then ignored.
- The nginx example below forwards `X-Forwarded-For`, so make sure the nginx
  host's IP (or the subnet it sits in) is in `trusted_ips`.

### Authentication (optional)

Set `auth.enabled: true` and configure `jwks_endpoint`/`issuer`/`audience` to require
a valid JWT (Bearer token) on every request. Disabled by default.

## RFC 9537 Redaction (2024 Profile)

The [2024 gTLD Response Profile](https://www.icann.org/en/contracted-parties/registry-operators/registration-data-access-protocol/gtld-rdap-profile-01-01-2020-en)
(§2.7.7/2.7.8, Appendix E) requires a `redacted` array whenever registrant/technical
personal data is withheld. The 2019-profile `redacted` *remarks* element is obsolete
under the 2024 profile.

The redaction extension itself is defined in [RFC 9537](https://www.rfc-editor.org/rfc/rfc9537).
Three methods are defined:

| Method | Use for | Example |
|--------|---------|---------|
| `emptyValue` | blanked `fn`, street, city, postal code | `"fn": [["fn", {}, "text", ""]]` |
| `replacementValue` | `email` replaced by an anonymized address or a `contact-uri` web form | |
| `removal` | handle/org/phone/fax removed entirely | |

Each entry carries a `name.type`, a JSONPath (`prePath`/`postPath`/`replacementPath`),
`pathLang: "jsonpath"`, `method`, and an optional `reason`:

```json
"redacted": [{
  "name": {"type": "Registrant Name"},
  "postPath": "$.entities[?(@.roles[0]=='registrant')].vcardArray[1][?(@[0]=='fn')][3]",
  "pathLang": "jsonpath",
  "method": "emptyValue",
  "reason": {"description": "Server policy"}
}]
```

### Object ID (handle) requirements

- A domain object must include a `handle` (Registry Domain ID / ROID), **or** omit it
  and provide a redaction entry of type `Registry Domain ID` with `method: "removal"`
  and `prePath: "$.handle"`.
- An entity object must include a `handle` (ROID or registrar contact ID), **or** omit
  it with the matching redaction entry.
- Handle suffixes must be repository IDs registered in the
  [IANA EPP Repository Identifiers registry](https://www.iana.org/assignments/epp-repository-ids/)
  to pass the EPPROID checks (`-46201`, `-47202`, `-63101`, …).

## Architecture

The server is layered so that the canonical registry data model is decoupled from
both storage and the RDAP wire format:

```
registry data model (internal/domain)   ← canonical, rich (contacts, history, audit,
        │                                  privacy, DNSSEC, registrar relationships,
        │                                  source-of-truth metadata)
        ▼
query service (internal/service)        ← maps domain aggregates → RDAP output
        │
        ▼
RDAP representation (internal/rdap)     ← pure RFC 9083 output DTOs (no storage)
        │
        ▼
HTTP handlers (internal/handlers)       ← request routing / response wrapping
```

- **`internal/domain`** — the authoritative registry model: `Domain`, `Contact`,
  `NameServer`, `IPNetwork`, `Autnum`, `Status`, `Event`, `SecureDNS`, plus
  `Metadata` (version, source-of-truth timestamps, history, audit) and privacy
  state. This is the model a real registry maps into.
- **`internal/service`** — the query/application service. It turns domain
  aggregates into RDAP objects and is the single seam where a registry can plug in
  its own model. The RDAP output here is validated against the ICANN conformance
  tool and is byte-compatible with the previously-passing output.
- **`internal/rdap`** — only the RFC 9083 wire types (domain, entity, nameserver,
  ip network, autnum, notices, links, conformance) plus notice/conformance
  builders. No storage types.
- **`internal/store`** — storage adapters (`memory`, `postgres`, `mysql`) that
  produce `domain` objects from the schema. You can keep using the default
  Postgres/MySQL schema as-is; the layering is purely additive.

The default `storage.driver` + shipped schemas work unchanged — no internal mapping
is required to just run the server. The `domain`/`service` layers exist so operators
with a different (richer) registry schema can map their model in without touching
the RDAP layer.

**Want concrete examples?** See the
[Architecture & Extension Guide](docs/ARCHITECTURE.md) — it walks through four
real usage scenarios:

| Scenario | You want to... | What you touch |
|----------|----------------|----------------|
| **A** "Just run it" | Serve RDAP over the shipped schema | nothing |
| **B** "My DB differs" | Point the server at your existing database | `store` (views or interface) |
| **C** "Richer model" | Keep history/privacy/audit that RDAP doesn't show | `domain` (already modeled) |
| **D** "Custom output" | Adjust the RDAP response | `service` (single place) |

## Project Structure

```
├── cmd/
│   └── rdapd/             # Main entry point (flag parsing, TLS, graceful shutdown)
├── internal/
│   ├── auth/              # JWT/Bearer authentication middleware
│   ├── config/            # YAML configuration + validation
│   ├── domain/            # Canonical registry data model (models, vcard, metadata)
│   ├── handlers/          # HTTP handlers (consume the query service)
│   ├── metrics/           # Prometheus metrics server
│   ├── middleware/        # Logging, security headers, content-type, rate limiting, client IP
│   ├── rdap/              # RFC 9083 output models + notice/conformance builders
│   ├── server/            # Chi router, middleware wiring, CORS
│   ├── service/           # Query service: domain model → RDAP output
│   └── store/             # Storage interface + memory, postgres & mysql implementations
├── migrations/            # PostgreSQL (001) and MySQL (002) schemas + seed data
├── examples/              # Example databases + configs (postgres/ and mysql/)
├── docs/                  # Guides (ARCHITECTURE.md: layering + usage scenarios)
├── config.yaml            # Example configuration (registrar mode)
├── Dockerfile             # Multi-stage Alpine build
├── docker-compose.yml     # Full stack (Postgres, MySQL, Prometheus, Grafana)
├── Makefile               # Build, test, lint, docker, migrate targets
├── prometheus.yml         # Metrics scrape config
└── go.mod                 # Module: github.com/tespio/go-rdap-server
```

## Testing & Coverage

Tests run in CI on every push/PR (see `.github/workflows/ci.yml`) and a
`coverage.out` artifact is uploaded to each run for inspection.

Current statement coverage (measured locally, `make test-cover`):

| Package | Coverage |
|---------|:---:|
| `internal/middleware` | 23.3% |
| `internal/store` | 6.2% |
| all other packages | 0% |
| **Total** | **6.7%** |

> Coverage is currently **low** — the conformance-critical `internal/service` and
> `internal/rdap` layers have no unit tests yet. The project is validated against
> the ICANN conformance tool (see [ICANN Conformance](#icann-conformance)), which
> exercises the full HTTP/RDAP output end-to-end, but statement-level unit tests
> for those packages are an open improvement.

```bash
make test          # go test -v -race ./...
make test-cover    # coverage report (coverage.html)
```

**Postgres integration test (read-consistency):** a test in `internal/store`
proves the `REPEATABLE READ` snapshot in `GetDomainAggregate` actually holds under a
concurrent write (a writer commits a registrar transfer mid-read and the reader must
still see the pre-write registrar). It is gated behind `RDAP_TEST_DSN` and skips
when unset, so CI is unaffected. To run it against a local Postgres:

```bash
RDAP_TEST_DSN="postgres://rdap:rdap@localhost:5432/rdap?sslmode=disable" \
  go test ./internal/store/ -run TestPostgresAggregateSnapshotIsCoherent -v
```

To report coverage to Codecov, enable Codecov for this repo and add the token as a
`CODECOV_TOKEN` secret; the CI workflow can then upload `coverage.out`.

## Development

```bash
make build        # build/rdapd
make run          # build + run with config.yaml
make test         # go test -v -race ./...
make test-cover   # coverage report (coverage.html)
make lint         # golangci-lint
make fmt          # gofmt
make tidy         # go mod tidy && go mod verify
make docker-build # docker build
make migrate-up   # apply PostgreSQL migrations
```

## License

MIT