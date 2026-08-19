# Architecture & Extension Guide

This guide explains the layered design and — most importantly — shows **concrete
ways to use it**, from "just run it" to "plug in your own registry model".

The core idea is stated in the README:

> The server is layered so that the canonical registry data model is decoupled
> from both storage and the RDAP wire format.

Below we unpack what that actually means with real examples.

---

## 1. The three layers in one diagram

```
            registry data model           query service            RDAP output
            (internal/domain)             (internal/service)       (internal/rdap)
            ┌──────────────────┐          ┌───────────────┐        ┌──────────────┐
            │ Domain           │          │  DomainToRDAP │        │ rdap.Domain  │
            │  Handle          │  ─────▶  │  EntityToRDAP │ ────▶  │ rdap.Entity  │
            │  Contacts{role}  │          │  NameserverTo │        │ rdap.Nameserver
            │  Nameservers[]   │          │  RDAP         │        │ rdap.IPNetwork
            │  Status[]        │          │  ...          │        │ rdap.Autnum  │
            │  Metadata{       │          └───────────────┘        └──────────────┘
            │    version,      │
            │    history,      │           storage adapters (internal/store)
            │    audit, ...}   │           memory · postgres · mysql
            └──────────────────┘
```

**Rule of thumb:** `internal/rdap` only knows how to *serialize*. `internal/domain`
only knows what a *registry object is*. `internal/service` translates one into the
other. `internal/store` decides *where the data lives*.

This means: **you can change any one layer without touching the others.**

---

## 2. Usage scenario A — "Just run it" (zero custom code)

You don't care about the layers at all. You want the server up against our shipped
Postgres/MySQL schema, serving ICANN-compliant RDAP.

```yaml
# config.yaml
storage:
  driver: "postgres"                       # or "mysql"
  dsn: "postgres://rdap:rdap@localhost:5432/rdap?sslmode=disable"
```

1. Load `migrations/001_init.sql` (or `examples/postgres/schema.sql` + `seed.sql`).
2. Run the server.
3. Done.

The store fills `domain` objects from those tables, the service maps them to RDAP,
and you get conformant output. **You never write a line of Go.** The layering is
invisible to you — it's purely internal.

---

## 3. Usage scenario B — "My database has a different schema"

You already have a real registry/registrar database with different table and column
names (say an EPP-style `registrations` table instead of `domains`). You want to keep
your database and still serve RDAP.

**Option B1 — SQL views (no Go code).** Because the stores read a fixed set of
columns from `domains`, `entities`, `nameservers`, etc., you can expose your existing
tables through views that match those names. See `examples/README.md`
("Mapping to your existing database") for a full worked example.

**Option B2 — implement the store interface (Go).** If views can't express your data
(e.g. it lives behind an API or in a very different shape), implement the small,
stable seam:

```go
// internal/store/store.go
type Interface interface {
    LookupDomain(name string) (*domain.Domain, error)
    LookupContact(handle string) (*domain.Contact, error)
    LookupNameserver(name string) (*domain.NameServer, error)
    LookupIPNetwork(cidr string) (*domain.IPNetwork, error)
    LookupAutnum(asn int) (*domain.Autnum, error)
    SearchDomainsByName(pattern string, limit int) ([]domain.Domain, error)
    // ... plus the other Search* methods, Ping, Close
}
```

Register it in `store.New()` next to `memory`/`postgres`/`mysql`, and select it with
`storage.driver`. Everything above the store (service → rdap → http) is reused
unchanged.

---

## 4. Usage scenario C — "I want richer registry semantics than RDAP exposes"

RDAP is a *read-only view*. A real registry has more going on: history, privacy,
audit, registrar relationships, source-of-truth versioning. Those live in
`internal/domain` and are **not** forced into the RDAP output.

```go
// internal/domain/models.go (abbreviated)
type Domain struct {
    Handle  string
    Status  []Status
    // Rich relationships:
    Contacts map[ContactRole][]string   // registrant, tech, admin, billing, registrar
    Registrar string                    // sponsoring registrar reference
    SecureDNS *SecureDNS                // DNSSEC DS + key records
    // Source-of-truth metadata:
    Metadata Metadata {
        Version   int64
        CreatedAt time.Time
        UpdatedAt time.Time
        UpdatedBy string
        Source    string                // "epp" | "srs" | ...
        History   []HistoryEntry        // prior versions (as-of queries)
        Audit     []AuditEntry
    }
}
```

This model can be richer than what RDAP shows. For example:

- You might store **multiple historical versions** (`Metadata.History`) to answer
  "what did this domain look like last month?", while RDAP only ever returns the
  current state.
- You might track **privacy state** per contact (public / redacted / proxy) and
  decide in the *service* whether to emit a `redacted` entry — without changing the
  wire types.
- You might record **who changed what** (`UpdatedBy`, `Audit`) for compliance, even
  though that data never appears in RDAP.

The point: the domain model is *yours*; the RDAP output is *theirs* (ICANN's). The
service is where you decide how the former maps to the latter.

---

## 5. Usage scenario D — "I need to customize the RDAP output"

Because all RDAP shaping happens in `internal/service`, that's the single place to
adjust output. Suppose your registrar wants a different `registrar expiration` event,
or an extra notice — you change the mapping there, and every handler benefits.

For example, the registrar `about` link points at your IANA-registered RDAP base URL
(set via `rdap.registrar_base_url`). The mapping that consumes it lives in
`service.DomainToRDAP`. If you needed to change how the registrar entity's vcard is
rendered, you'd edit it in one place and all lookups + searches would reflect it.

> **Conformance note:** the default mapping in `service` reproduces the exact output
> that passes the ICANN tool (STD 95, 2019 registrar, 2024 registrar — all 0 errors).
> If you customize the mapping, re-run the conformance suite in `.rdapct/` to confirm
> you haven't broken compliance.

### Read consistency (transactions)

An RDAP domain response composes data from multiple objects (the domain row, its
sponsoring registrar, its contacts, its nameservers). If those were read with
independent queries, a concurrent update could produce a **torn response** — e.g. a
domain that says "transferred to Registrar B" while still carrying Registrar A's
contact/abuse data. That violates the spec, which requires the registrar entity to
be internally coherent.

To prevent this, the server reads domain responses through a **single transactional
aggregate**:

- `store.GetDomainAggregate(name)` returns a `domain.DomainAggregate` — the domain
  plus its resolved registrar, contacts, and nameservers — read from **one snapshot**.
- The memory store holds a single read lock across the whole resolution.
- Postgres uses `BEGIN ... ISOLATION LEVEL REPEATABLE READ`; MySQL uses
  `REPEATABLE READ` (read-only) — a consistent snapshot is sufficient for read-only
  composition; serializable is not required.
- `service.LookupDomain` renders the RDAP response from that aggregate, so status,
  events, and the embedded registrar/contacts/nameservers all reflect the same
  moment in time.

> If you add a new store or change how a domain response is composed, always resolve
> all of its related objects inside the same transaction/snapshot, rather than with
> separate `Lookup*` calls, or staleness can sneak back in.

---

## 6. Why this matters for integration

| Concern | Without this layering | With it |
|---------|----------------------|---------|
| Use the shipped schema | Works | Works (unchanged) |
| Point at my existing schema | Rewrite the HTTP/response code | Add a store (B1/B2) or views |
| Richer model than RDAP | Squeeze it into RDAP-shaped records | Keep it in `domain`, map in `service` |
| Custom RDAP output | Hunt through handlers | Edit one place in `service` |
| Add history/privacy/audit | Fight the CRUD-shaped records | Already modeled in `domain` |

---

## 7. Package map

```
internal/domain    models.go   rich registry aggregates + Metadata
                   vcard.go    structured jCard + RFC 9537 Redaction
internal/service   service.go  domain → rdap mapping (the seam)
internal/rdap      models.go   RFC 9083 output DTOs only
                   rdap.go     notice/conformance builders, helpers
internal/store     store.go    Interface (produces domain objects)
                   memory.go   in-memory store (seeded)
                   postgres.go pgx store
                   mysql.go    MySQL store
                   mapping.go  JSON ↔ domain mapping helpers
internal/handlers  handlers.go HTTP handlers (consume service)
internal/server    server.go   router + middleware wiring
```

---

## 8. Quick decisions

- **"I just want RDAP over our existing DB"** → Scenario B1 (views) or B2 (store).
- **"I need history / privacy / audit"** → Scenario C; the model already supports it.
- **"I want to tweak the output"** → Scenario D; edit `service`.
- **"I don't care, ship it"** → Scenario A; nothing to do.
