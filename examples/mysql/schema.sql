-- RDAP Server — MySQL 8 example schema
-- A self-contained schema for the domain example database.
-- Load before seed.sql:
--   mysql -u rdap -p < examples/mysql/schema.sql

CREATE DATABASE IF NOT EXISTS rdap CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
USE rdap;

-- Entities (registrars, registrants, contacts)
CREATE TABLE IF NOT EXISTS entities (
    handle      VARCHAR(80) PRIMARY KEY,
    vcard_json  LONGTEXT,
    roles       JSON NOT NULL,
    status      JSON NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    public_ids  JSON NOT NULL
);

-- Nameservers
CREATE TABLE IF NOT EXISTS nameservers (
    handle       VARCHAR(80) PRIMARY KEY,
    ldh_name     VARCHAR(253) NOT NULL UNIQUE,
    unicode_name VARCHAR(253) NOT NULL,
    ipv4         JSON,
    ipv6         JSON,
    status       JSON NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE INDEX idx_nameservers_ldh_name ON nameservers (ldh_name);

-- Domains
CREATE TABLE IF NOT EXISTS domains (
    handle       VARCHAR(80) PRIMARY KEY,
    ldh_name     VARCHAR(253) NOT NULL UNIQUE,
    unicode_name VARCHAR(253) NOT NULL,
    tld          VARCHAR(63) NOT NULL,
    status       JSON NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    expires_at   TIMESTAMP NULL,
    registrant   VARCHAR(80) NULL,
    admin        VARCHAR(80) NULL,
    tech         VARCHAR(80) NULL,
    billing      VARCHAR(80) NULL,
    nameservers  JSON NOT NULL,
    secure_dns   JSON NULL
);

CREATE INDEX idx_domains_ldh_name ON domains (ldh_name);
CREATE INDEX idx_domains_tld ON domains (tld);
CREATE INDEX idx_domains_expires_at ON domains (expires_at);

-- Domain-Nameserver junction
CREATE TABLE IF NOT EXISTS domain_nameservers (
    domain_handle VARCHAR(80) NOT NULL,
    ns_handle     VARCHAR(80) NOT NULL,
    PRIMARY KEY (domain_handle, ns_handle),
    CONSTRAINT fk_dn_domain FOREIGN KEY (domain_handle) REFERENCES domains(handle) ON DELETE CASCADE,
    CONSTRAINT fk_dn_ns FOREIGN KEY (ns_handle) REFERENCES nameservers(handle) ON DELETE CASCADE
);

CREATE INDEX idx_domain_ns_ns ON domain_nameservers (ns_handle);

-- IP Networks.
-- start_ip/end_ip = IPv4 range as UNSIGNED BIGINT (INET_ATON value);
-- start_ip6/end_ip6 = IPv6 range as 16-byte big-endian VARBINARY.
-- MySQL has no native CIDR type, so the server matches on these ranges.
CREATE TABLE IF NOT EXISTS ip_networks (
    handle        VARCHAR(80) PRIMARY KEY,
    start_address VARCHAR(45) NOT NULL,
    end_address   VARCHAR(45) NOT NULL,
    ip_version    VARCHAR(2) NOT NULL,
    start_ip      BIGINT UNSIGNED NULL,
    end_ip        BIGINT UNSIGNED NULL,
    start_ip6     VARBINARY(16) NULL,
    end_ip6       VARBINARY(16) NULL,
    cidr          JSON NOT NULL,
    name          VARCHAR(255),
    type          VARCHAR(32),
    country       VARCHAR(2),
    status        JSON NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    parent_handle VARCHAR(80) NULL,
    CONSTRAINT chk_ip_version CHECK (ip_version IN ('v4', 'v6'))
);

CREATE INDEX idx_ip_networks_v4 ON ip_networks (ip_version, start_ip, end_ip);
CREATE INDEX idx_ip_networks_v6 ON ip_networks (ip_version, start_ip6, end_ip6);

-- Autonomous System Numbers
CREATE TABLE IF NOT EXISTS autnums (
    handle     VARCHAR(80) PRIMARY KEY,
    start_asn  BIGINT NOT NULL,
    end_asn    BIGINT NOT NULL,
    name       VARCHAR(255),
    type       VARCHAR(32),
    country    VARCHAR(2),
    status     JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);