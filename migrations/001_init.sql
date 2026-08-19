-- RDAP Server Database Schema
-- Implements storage for domain registration data per RFC 7483/9083

BEGIN;

-- Domains table
CREATE TABLE IF NOT EXISTS domains (
    handle          TEXT PRIMARY KEY,
    ldh_name        TEXT NOT NULL UNIQUE,
    unicode_name    TEXT NOT NULL,
    tld             TEXT NOT NULL,
    status          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    registrant      TEXT REFERENCES entities(handle),
    admin           TEXT REFERENCES entities(handle),
    tech            TEXT REFERENCES entities(handle),
    billing         TEXT REFERENCES entities(handle),
    nameservers     JSONB NOT NULL DEFAULT '[]'::jsonb,
    secure_dns      JSONB
);

CREATE INDEX idx_domains_ldh_name ON domains (ldh_name);
CREATE INDEX idx_domains_tld ON domains (tld);
CREATE INDEX idx_domains_expires_at ON domains (expires_at);

-- Entities table (registrars, registrants, contacts)
CREATE TABLE IF NOT EXISTS entities (
    handle      TEXT PRIMARY KEY,
    vcard_json  TEXT,
    roles       JSONB NOT NULL DEFAULT '[]'::jsonb,
    status      JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    public_ids  JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX idx_entities_handle ON entities (handle);

-- Nameservers table
CREATE TABLE IF NOT EXISTS nameservers (
    handle       TEXT PRIMARY KEY,
    ldh_name     TEXT NOT NULL UNIQUE,
    unicode_name TEXT NOT NULL,
    ipv4         JSONB NOT NULL DEFAULT '[]'::jsonb,
    ipv6         JSONB NOT NULL DEFAULT '[]'::jsonb,
    status       JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nameservers_ldh_name ON nameservers (ldh_name);

-- Domain-Nameserver junction table
CREATE TABLE IF NOT EXISTS domain_nameservers (
    domain_handle TEXT NOT NULL REFERENCES domains(handle) ON DELETE CASCADE,
    ns_handle     TEXT NOT NULL REFERENCES nameservers(handle) ON DELETE CASCADE,
    PRIMARY KEY (domain_handle, ns_handle)
);

CREATE INDEX idx_domain_ns_ns ON domain_nameservers (ns_handle);

-- IP Networks table
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
    parent_handle TEXT REFERENCES ip_networks(handle)
);

CREATE INDEX idx_ip_networks_cidr ON ip_networks USING GIST (cidr);

-- Autonomous System Numbers
CREATE TABLE IF NOT EXISTS autnums (
    handle      TEXT PRIMARY KEY,
    start_asn   BIGINT NOT NULL,
    end_asn     BIGINT NOT NULL,
    name        TEXT,
    type        TEXT,
    country     TEXT,
    status      JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    id          BIGSERIAL PRIMARY KEY,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    remote_addr TEXT NOT NULL,
    user_agent  TEXT,
    status_code INT NOT NULL,
    duration_ms INT NOT NULL
);

CREATE INDEX idx_audit_log_timestamp ON audit_log (timestamp);
CREATE INDEX idx_audit_log_path ON audit_log (path);

-- Seed data for testing (handles use the <local>-<EPPROID> ROID form)
INSERT INTO entities (handle, roles, status) VALUES
    ('2', '["registrar"]', '["active"]'),
    ('888', '["technical"]', '["active"]'),
    ('REG1-NAME', '["registrant"]', '["active"]')
ON CONFLICT (handle) DO NOTHING;

INSERT INTO nameservers (handle, ldh_name, unicode_name, ipv4, ipv6, status) VALUES
    ('NS1-NAME', 'ns1.example.com', 'ns1.example.com', '["8.8.8.8"]', '["2001:4860:4860::8888"]', '["associated"]'),
    ('NS2-NAME', 'ns2.example.com', 'ns2.example.com', '["1.1.1.1"]', '["2606:4700:4700::1111"]', '["associated"]')
ON CONFLICT (handle) DO NOTHING;

INSERT INTO domains (handle, ldh_name, unicode_name, tld, status, expires_at, registrant, admin, tech)
VALUES (
    'EX1-NAME',
    'example.com',
    'example.com',
    'com',
    '["active"]',
    NOW() + INTERVAL '1 year',
    '2',
    '888',
    '888'
)
ON CONFLICT (handle) DO NOTHING;

INSERT INTO domain_nameservers (domain_handle, ns_handle) VALUES
    ('EX1-NAME', 'NS1-NAME'),
    ('EX1-NAME', 'NS2-NAME')
ON CONFLICT DO NOTHING;

COMMIT;
