-- RDAP Server — PostgreSQL example schema
-- A self-contained schema for the domain example database.
-- Load before seed.sql.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Entities (registrars, registrants, contacts)
CREATE TABLE IF NOT EXISTS entities (
    handle      TEXT PRIMARY KEY,
    vcard_json  TEXT,
    roles       JSONB NOT NULL DEFAULT '[]'::jsonb,
    status      JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    public_ids  JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Registry metadata
    version     BIGINT NOT NULL DEFAULT 1,
    updated_by  TEXT,
    source      TEXT DEFAULT 'rdap'
);

CREATE INDEX idx_entities_handle ON entities (handle);

-- Nameservers
CREATE TABLE IF NOT EXISTS nameservers (
    handle       TEXT PRIMARY KEY,
    ldh_name     TEXT NOT NULL UNIQUE,
    unicode_name TEXT NOT NULL,
    ipv4         JSONB NOT NULL DEFAULT '[]'::jsonb,
    ipv6         JSONB NOT NULL DEFAULT '[]'::jsonb,
    status       JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Registry metadata
    version      BIGINT NOT NULL DEFAULT 1,
    updated_by   TEXT,
    source       TEXT DEFAULT 'rdap'
);

CREATE INDEX idx_nameservers_ldh_name ON nameservers (ldh_name);

-- Domains
CREATE TABLE IF NOT EXISTS domains (
    handle       TEXT PRIMARY KEY,
    ldh_name     TEXT NOT NULL UNIQUE,
    unicode_name TEXT NOT NULL,
    tld          TEXT NOT NULL,
    status       JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ,
    registrant   TEXT REFERENCES entities(handle),
    admin        TEXT REFERENCES entities(handle),
    tech         TEXT REFERENCES entities(handle),
    billing      TEXT REFERENCES entities(handle),
    nameservers  JSONB NOT NULL DEFAULT '[]'::jsonb,
    secure_dns   JSONB,
    -- Registry metadata
    version      BIGINT NOT NULL DEFAULT 1,
    updated_by   TEXT,
    source       TEXT DEFAULT 'rdap'
);

CREATE INDEX idx_domains_ldh_name ON domains (ldh_name);
CREATE INDEX idx_domains_tld ON domains (tld);
CREATE INDEX idx_domains_expires_at ON domains (expires_at);

-- Domain-Nameserver junction
CREATE TABLE IF NOT EXISTS domain_nameservers (
    domain_handle TEXT NOT NULL REFERENCES domains(handle) ON DELETE CASCADE,
    ns_handle     TEXT NOT NULL REFERENCES nameservers(handle) ON DELETE CASCADE,
    PRIMARY KEY (domain_handle, ns_handle)
);

CREATE INDEX idx_domain_ns_ns ON domain_nameservers (ns_handle);

-- IP Networks
CREATE TABLE IF NOT EXISTS ip_networks (
    handle        TEXT PRIMARY KEY,
    start_address INET NOT NULL,
    end_address   INET NOT NULL,
    ip_version    TEXT NOT NULL CHECK (ip_version IN ('v4', 'v6')),
    cidr          TEXT[] NOT NULL,
    name          TEXT,
    type          TEXT,
    country       TEXT,
    status        JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_handle TEXT REFERENCES ip_networks(handle),
    -- Registry metadata
    version       BIGINT NOT NULL DEFAULT 1,
    updated_by    TEXT,
    source        TEXT DEFAULT 'rdap'
);

CREATE INDEX idx_ip_networks_cidr ON ip_networks (ip_version);

-- Autonomous System Numbers
CREATE TABLE IF NOT EXISTS autnums (
    handle     TEXT PRIMARY KEY,
    start_asn  BIGINT NOT NULL,
    end_asn    BIGINT NOT NULL,
    name       TEXT,
    type       TEXT,
    country    TEXT,
    status     JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Registry metadata
    version    BIGINT NOT NULL DEFAULT 1,
    updated_by TEXT,
    source     TEXT DEFAULT 'rdap'
);

-- Registry object history (versioned snapshots for "as-of" queries)
CREATE TABLE IF NOT EXISTS registry_history (
    id          BIGSERIAL PRIMARY KEY,
    object_type TEXT NOT NULL,
    object_id   TEXT NOT NULL,
    version     BIGINT NOT NULL,
    action      TEXT,
    actor       TEXT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    snapshot    JSONB,
    UNIQUE (object_type, object_id, version)
);

CREATE INDEX idx_registry_history_object ON registry_history (object_type, object_id);
CREATE INDEX idx_registry_history_changed_at ON registry_history (changed_at);