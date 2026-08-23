# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
- **Major test coverage expansion** — statement coverage raised from ~24% to **50.2%**:
  `config` 100%, `service` 94.6%, `handlers` 62.8%, `store` 33.9% (unit-testable parts).
  Added tests for store JSON mapping + in-memory store, service lookup/search wrappers,
  HTTP lookup handlers, `/help`, and config load/validate/defaults.

### Fixed
- **`/ip/{network}` CIDR routing** — the route used chi's single-segment `{network}`
  param, so `/ip/8.8.8.0/24` (CIDR contains a slash) returned 404. Now uses `/ip/*`
  with the full captured path, so CIDR lookups work as documented.

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

[Unreleased]: https://github.com/tespio/go-rdap-server/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/tespio/go-rdap-server/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/tespio/go-rdap-server/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/tespio/go-rdap-server/releases/tag/v1.0.0
