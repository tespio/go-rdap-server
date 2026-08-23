# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.1] - 2026-08-23

### Added
- **Legacy WHOIS gateway (RFC 3912)** — optional `whois.enabled: true` port 43
  server that answers plain-text WHOIS queries rendered from the **same registry
  data** the RDAP endpoints serve, so one binary replaces both the WHOIS and RDAP
  services during the RDAP migration. Supports bare and keyword query forms
  (`domain example.com`, `ns ...`, `entity ...`, `ip ...`, `asn ...`); domains are
  rendered today, other object types return an explanation pointing at the RDAP
  service, and unknown objects return `NOT FOUND`. New `whois.enabled` /
  `whois.port` config keys (default port 43).
- **OAuth 2.0 / OpenID Connect authentication (RFC 9560 `farv1`)** — the auth
  middleware now **verifies the JWT access-token signature** against the
  authorization server's JWKS (RS256/384/512, ES256/384/512, PS256/384/512) in
  addition to validating `iss`/`aud`/`exp`/`iat`. Previously only claims were
  checked, so a forged token with the right issuer/audience would be accepted.
  New `auth.algorithms` restricts accepted algorithms; `jwks_endpoint` defaults
  to `<issuer>/.well-known/jwks.json`. On failure the server returns `401` with a
  `WWW-Authenticate: Bearer` challenge (RFC 6750). When auth is enabled, `/help`
  advertises `farv1_openidcConfiguration` + `farv1` conformance (token-oriented
  clients), and the RFC 9560 `rdap_allowed_purposes` / `rdap_dnt_allowed` claims
  are parsed.
- **Optional IANA RDAP extensions via `rdap.extensions`** — config-gated "extras"
  that append their extension identifier to `rdapConformance` and emit the
  extension's JSON members:
  - `ttl0` — DNS TTL values (`ttl0_data`) on domain/nameserver objects
    (draft-ietf-regext-rdap-ttl-extension), configured via `rdap.ttl0`.
  - `geofeed1` — `rel=geofeed` link (RFC 9877) on IP network objects, configured
    via `rdap.geofeed.url`.
  - `cidr0` — `cidr0_cidrs` array (NRO) on IP network objects.
  - `reverse_search` — `GET /domains/reverse_search/entity` (RFC 9536) with
    `handle`/`role`/`fn`/`email` predicates, help `reverse_search_properties`,
    and per-response `reverse_search_properties_mapping`. Implemented on the
    in-memory store; PostgreSQL/MySQL return 501 (RFC 9536 §7).
  - Unknown extension identifiers are rejected at config load.
  - **Verified conformance impact** (ICANN rdapct v3.1.0, 2024 registrar):
    `geofeed1`, `cidr0`, and `reverse_search` keep 78 groups / 0 errors;
    `ttl0` **breaks conformance** (`-12208`, rdapct rejects `ttl0_data` on
    nameservers because the draft isn't in its allowed-members schema). All
    extensions default to OFF, so the CI conformance gate is unaffected.
- **Client-side RDAP lookup page** — a dependency-free, single-file browser UI at
  `web/index.html` ("whois, modern"): auto-detecting lookups for domains, nameservers,
  entities, IP networks (v4 + v6), and ASNs; IANA bootstrap resolution (RFC 7484) to
  find the authoritative server; clean jCard/vcard rendering; and a raw-JSON view.
  Works against this server out of the box (CORS is enabled by default).
- **`rdap.search_enabled` config flag** — controls the RFC 7482 search endpoints
  (`/domains?name=*`, `/entities?fn=*`, `/nameservers?name=*`). **Disabled by default**:
  wildcard searches are an abuse/DoS vector and most registrars/registries don't offer
  them. When disabled, search routes return **HTTP 501 Not Implemented** (RFC 9082 §5.1).
  Set `search_enabled: true` to enable searches.
- **Search-flag tests** — unit tests cover both enabled (200 + results) and disabled
  (501) behavior, including HEAD parity.
- **`/help` advertises disabled searches** — when `rdap.search_enabled: false` (the
  default), the `/help` response includes a "Search Disabled" notice documenting that
  search queries are unavailable, mirroring how enforced rate limits are advertised.
- **Major test coverage expansion** — statement coverage raised from ~24% to **92.9%**
  (with the PostgreSQL + MySQL integration tests; 70.7% without a DB). Per-package:
  `config` 100%, `metrics` 100%, `middleware` 99%, `service` 98.7%, `rdap` 97.5%,
  `auth` 96.7%, `store` 93.1%, `handlers` 87.8%, `server` 84.6%, `cmd/rdapd` 55.8%.
  Added tests across every layer including comprehensive **PostgreSQL and MySQL
  integration tests** covering all lookups, searches, the transactional aggregate,
  IP networks (v4 + v6), autnums, and the seed fixtures. CI now runs the store
  integration tests against fresh PostgreSQL and MySQL instances.
- **Dependency refresh** — all modules bumped to patched versions (x/crypto v0.55,
  x/net v0.58, pgx/v5 v5.10, chi v5.3, prometheus client_golang v1.24, mysql v1.10,
  edwards25519 v1.2, …) fixing 25 Dependabot alerts. Module now requires Go 1.25.

### Fixed
- **`/ip/{network}` CIDR routing** — the route used chi's single-segment `{network}`
  param, so `/ip/8.8.8.0/24` (CIDR contains a slash) returned 404. Now uses `/ip/*`
  with the full captured path, so CIDR lookups work as documented.
- **JWT claim base64 decoding** — replaced the hand-rolled decoder (which emitted a
  trailing NUL byte) with `base64.RawURLEncoding`, the correct decoder for JWT
  payloads.
- **Nil `metricsSrv` shutdown panic** — the old `main()` called
  `metricsSrv.Shutdown` unconditionally, panicking when metrics were disabled.
  `cmd/rdapd` now guards against a nil metrics server.
- **PostgreSQL IP-network lookup** — `LookupIPNetwork` scanned the `inet` columns into
  `string` and the `TEXT[]` `cidr` column into `[]byte`, and compared against the
  untyped array; all three failed at runtime. Now casts `inet` to text, scans `cidr`
  into `[]string`, and compares `$1::inet <<= ANY(cidr::inet[])`. The integration
  tests caught and verify this.
- **PostgreSQL contact search wildcards** — `SearchContactsByName` didn't translate
  `*`/`?` glob wildcards into SQL `%`/`_` (unlike every other search), so patterns
  like `REG1*` never matched. Now applies `patternToSQL`.
- **MySQL contact search wildcards** — same fix as PostgreSQL: `SearchContactsByName`
  now applies `patternToSQL` so `*`/`?` globs match in MySQL `LIKE` queries.

## [1.2.0] - 2026-08-19

### Added
- **Configurable Terms of Service + custom RDAP notices** — set `rdap.tos.{title,description,url}`
  to customize the Terms of Service card/link, and `rdap.custom_notices[]` to append
  registrar/registry-specific notices. ICANN-mandated notices are always preserved.
- **`/help` documents the enforced rate limit** — the Rate Limiting notice now reports the
  actual configured per-IP limits (requests/window/burst). `/help` also uses a generic
  Terms of Service notice rather than domain-specific text.
- **Transactional domain aggregate read** — `store.GetDomainAggregate` resolves a domain
  plus its registrar, contacts, and nameservers from a single `REPEATABLE READ` snapshot,
  so a concurrent transfer/renewal/delete can never produce a torn RDAP response.
- **Fresh-database migration tests in CI** — both PostgreSQL and MySQL migrations are
  applied against a clean database on every push to catch schema regressions.
- **ICANN RDAPCT conformance job in CI** — builds the server, starts it with TLS, and runs
  the official ICANN RDAP Conformance Tool (gTLT Registrar, 2024 profile), failing the
  build if conformance regresses (verified 78 groups / 0 errors).
- **Postgres read-consistency integration test** — proves the `REPEATABLE READ` snapshot
  actually holds under a concurrent write.

### Changed
- **Layered architecture** — introduced `internal/domain` (canonical registry data model)
  and `internal/service` (query service), decoupling the registry model from both storage
  and the RDAP wire format. Stores now produce domain objects; `internal/rdap` is pure
  output DTOs. See `docs/ARCHITECTURE.md`.
- **Module path** corrected to `github.com/tespio/go-rdap-server`.
- **`server_name`/version defaults** bumped to `1.2.0`.

### Fixed
- **CodeQL high: unsafe `uint64 -> int` ASN conversion** — ASN fields are now `uint32`,
  parsed with `bitSize=32`, eliminating potential integer overflow on 32-bit builds.
- **Sparse/name-less vcard** — `vcardToJCard` always emits the REQUIRED jCard `fn`,
  synthesizing it from the contact handle when absent, so a registrar/registrant with no
  name can never produce an invalid (fn-less) vcard.
- **PostgreSQL migration** — create `entities` before `domains` (FK ordering) and replace
  the non-portable GIST index on `text[]` with a plain B-tree index.
- **MySQL migration** — prefix index on `audit_log.path` (was over the 3072-byte utf8mb4
  limit) and a default for `entities.public_ids`.
- **Metrics server nil-pointer crash** — the server no longer segfaults at startup when
  `metrics.enabled` is false.
- **`/help` showing domain-specific text** — now uses a generic Terms of Service notice.

### Security
- **Trusted-proxy client IP resolution** — `X-Forwarded-For`/`X-Real-IP` are only honored
  when the direct peer is in `rate_limiting.trusted_ips`, preventing clients from spoofing
  their IP to bypass per-IP rate limiting.

### Documentation
- Added `docs/ARCHITECTURE.md` (layering + usage scenarios), Testing & Coverage section,
  and ICANN Conformance CI badge/note.

## [1.1.0] - 2026-08-19

### Added
- **Registry metadata + history schema** — `version`/`updated_by`/`source` columns and a
  `registry_history` table in both PostgreSQL and MySQL migrations.
- **Architecture & Extension Guide** (`docs/ARCHITECTURE.md`) with concrete usage scenarios.

### Changed
- **Layered architecture introduced** — `internal/domain` + `internal/service` decouple the
  registry model from storage and the RDAP wire format.
- Version bumped to `1.1.0`.

### Fixed
- **Domain status per RFC 8056** — domains use `active` (EPP `ok`); `associated` is only
  valid for nameservers/contacts.

### Security
- **Trusted-proxy client IP resolution** for rate limiting (spoof-proof `X-Forwarded-For`).

## [1.0.0] - 2026-08-19

Initial release.

### Added
- RDAP server (RFC 9082/9083, STD 95) with lookups and searches for domains, entities,
  nameservers, IP networks, and autnums.
- 2019 & 2024 gTLD RDAP Profiles, dual registrar/registry operator modes.
- IDN support (IDNA 2008), DNSSEC, jCard vCards.
- Storage backends: in-memory, PostgreSQL, and MySQL.
- Rate limiting (per-IP), optional JWT auth, Prometheus metrics, CORS, TLS.
- Docker + docker-compose, Makefile, CI (build/vet/test).
- Example databases and schemas under `examples/`.

[Unreleased]: https://github.com/tespio/go-rdap-server/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/tespio/go-rdap-server/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/tespio/go-rdap-server/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/tespio/go-rdap-server/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/tespio/go-rdap-server/releases/tag/v1.0.0
