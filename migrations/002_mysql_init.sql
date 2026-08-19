-- RDAP Server MySQL Schema (MySQL 8.0+)
-- Implements storage for domain registration data per RFC 7483/9083.
-- Mirrors migrations/001_init.sql (PostgreSQL) using MySQL types and functions.

CREATE DATABASE IF NOT EXISTS rdap CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
USE rdap;

-- Domains table
CREATE TABLE IF NOT EXISTS domains (
    handle          VARCHAR(80) PRIMARY KEY,
    ldh_name        VARCHAR(253) NOT NULL UNIQUE,
    unicode_name    VARCHAR(253) NOT NULL,
    tld             VARCHAR(63) NOT NULL,
    status          JSON NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    expires_at      TIMESTAMP NULL,
    registrant      VARCHAR(80) NULL,
    admin           VARCHAR(80) NULL,
    tech            VARCHAR(80) NULL,
    billing         VARCHAR(80) NULL,
    nameservers     JSON NOT NULL,
    secure_dns      JSON NULL,
    -- Registry metadata
    version         BIGINT NOT NULL DEFAULT 1,
    updated_by      VARCHAR(255) NULL,
    source          VARCHAR(64) DEFAULT 'rdap'
);

CREATE INDEX idx_domains_ldh_name ON domains (ldh_name);
CREATE INDEX idx_domains_tld ON domains (tld);
CREATE INDEX idx_domains_expires_at ON domains (expires_at);

-- Entities table (registrars, registrants, contacts)
CREATE TABLE IF NOT EXISTS entities (
    handle      VARCHAR(80) PRIMARY KEY,
    vcard_json  LONGTEXT,
    roles       JSON NOT NULL,
    status      JSON NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    public_ids  JSON NOT NULL,
    -- Registry metadata
    version     BIGINT NOT NULL DEFAULT 1,
    updated_by  VARCHAR(255) NULL,
    source      VARCHAR(64) DEFAULT 'rdap'
);

-- Nameservers table
CREATE TABLE IF NOT EXISTS nameservers (
    handle       VARCHAR(80) PRIMARY KEY,
    ldh_name     VARCHAR(253) NOT NULL UNIQUE,
    unicode_name VARCHAR(253) NOT NULL,
    ipv4         JSON,
    ipv6         JSON,
    status       JSON NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    -- Registry metadata
    version      BIGINT NOT NULL DEFAULT 1,
    updated_by   VARCHAR(255) NULL,
    source       VARCHAR(64) DEFAULT 'rdap'
);

CREATE INDEX idx_nameservers_ldh_name ON nameservers (ldh_name);

-- Domain-Nameserver junction table
CREATE TABLE IF NOT EXISTS domain_nameservers (
    domain_handle VARCHAR(80) NOT NULL REFERENCES domains(handle) ON DELETE CASCADE,
    ns_handle     VARCHAR(80) NOT NULL REFERENCES nameservers(handle) ON DELETE CASCADE,
    PRIMARY KEY (domain_handle, ns_handle)
);

CREATE INDEX idx_domain_ns_ns ON domain_nameservers (ns_handle);

-- IP Networks table.
-- start_ip/end_ip hold the IPv4 range as UNSIGNED BIGINT (INET_ATON value) and
-- start_ip6/end_ip6 hold the IPv6 range as 16-byte big-endian values, because
-- MySQL has no native CIDR/inet type. The server computes the queried range and
-- compares numerically.
CREATE TABLE IF NOT EXISTS ip_networks (
    handle        VARCHAR(80) PRIMARY KEY,
    start_address VARCHAR(45) NOT NULL,
    end_address   VARCHAR(45) NOT NULL,
    ip_version    VARCHAR(2) NOT NULL CHECK (ip_version IN ('v4', 'v6')),
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
    parent_handle VARCHAR(80) NULL REFERENCES ip_networks(handle),
    -- Registry metadata
    version       BIGINT NOT NULL DEFAULT 1,
    updated_by    VARCHAR(255) NULL,
    source        VARCHAR(64) DEFAULT 'rdap'
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
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    -- Registry metadata
    version    BIGINT NOT NULL DEFAULT 1,
    updated_by VARCHAR(255) NULL,
    source     VARCHAR(64) DEFAULT 'rdap'
);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    timestamp   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    method      VARCHAR(8) NOT NULL,
    path        VARCHAR(1024) NOT NULL,
    remote_addr VARCHAR(45) NOT NULL,
    user_agent  VARCHAR(512),
    status_code INT NOT NULL,
    duration_ms INT NOT NULL
);

CREATE INDEX idx_audit_log_timestamp ON audit_log (timestamp);
CREATE INDEX idx_audit_log_path ON audit_log (path);

-- Registry object history (versioned snapshots for "as-of" queries)
CREATE TABLE IF NOT EXISTS registry_history (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    object_type VARCHAR(64) NOT NULL,
    object_id   VARCHAR(255) NOT NULL,
    version     BIGINT NOT NULL,
    action      VARCHAR(64),
    actor       VARCHAR(255),
    changed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    snapshot    JSON NULL,
    UNIQUE KEY uq_registry_history (object_type, object_id, version)
);

CREATE INDEX idx_registry_history_object ON registry_history (object_type, object_id);
CREATE INDEX idx_registry_history_changed_at ON registry_history (changed_at);

-- Seed data for testing (handles use the <local>-<EPPROID> ROID form)
INSERT INTO entities (handle, roles, status) VALUES
    ('2', '["registrar"]', '["active"]'),
    ('888', '["technical"]', '["active"]'),
    ('REG1-NAME', '["registrant"]', '["active"]')
ON DUPLICATE KEY UPDATE handle = VALUES(handle);

INSERT INTO nameservers (handle, ldh_name, unicode_name, ipv4, ipv6, status) VALUES
    ('NS1-NAME', 'ns1.example.com', 'ns1.example.com', '["8.8.8.8"]', '["2001:4860:4860::8888"]', '["associated"]'),
    ('NS2-NAME', 'ns2.example.com', 'ns2.example.com', '["1.1.1.1"]', '["2606:4700:4700::1111"]', '["associated"]')
ON DUPLICATE KEY UPDATE handle = VALUES(handle);

INSERT INTO domains (handle, ldh_name, unicode_name, tld, status, expires_at, registrant, admin, tech, nameservers, secure_dns)
VALUES (
    'EX1-NAME',
    'example.com',
    'example.com',
    'com',
    '["active"]',
    DATE_ADD(NOW(), INTERVAL 1 YEAR),
    '2',
    '888',
    '888',
    JSON_ARRAY(
        JSON_OBJECT(
            'handle', 'NS1-NAME', 'ldhName', 'ns1.example.com', 'unicodeName', 'ns1.example.com',
            'ipv4', JSON_ARRAY('8.8.8.8'), 'ipv6', JSON_ARRAY('2001:4860:4860::8888'),
            'status', JSON_ARRAY('associated')
        ),
        JSON_OBJECT(
            'handle', 'NS2-NAME', 'ldhName', 'ns2.example.com', 'unicodeName', 'ns2.example.com',
            'ipv4', JSON_ARRAY('1.1.1.1'), 'ipv6', JSON_ARRAY('2606:4700:4700::1111'),
            'status', JSON_ARRAY('associated')
        )
    ),
    JSON_OBJECT('zoneSigned', FALSE, 'delegationSigned', FALSE)
)
ON DUPLICATE KEY UPDATE handle = VALUES(handle);

INSERT INTO domain_nameservers (domain_handle, ns_handle) VALUES
    ('EX1-NAME', 'NS1-NAME'),
    ('EX1-NAME', 'NS2-NAME')
ON DUPLICATE KEY UPDATE domain_handle = VALUES(domain_handle);

-- Sample IP network (8.8.8.0/24 => 8.8.8.0 .. 8.8.8.255)
INSERT INTO ip_networks (handle, start_address, end_address, ip_version, start_ip, end_ip, cidr, name, type, country, status)
VALUES (
    'NET-8-8-8-0-24',
    '8.8.8.0',
    '8.8.8.255',
    'v4',
    INET_ATON('8.8.8.0'),
    INET_ATON('8.8.8.255'),
    JSON_ARRAY('8.8.8.0/24'),
    'GOOGLE',
    'ALLOCATED',
    'US',
    '["active"]'
)
ON DUPLICATE KEY UPDATE handle = VALUES(handle);

INSERT INTO autnums (handle, start_asn, end_asn, name, type, country, status)
VALUES ('AS15169', 15169, 15169, 'GOOGLE', 'DIRECT ALLOCATION', 'US', '["active"]')
ON DUPLICATE KEY UPDATE handle = VALUES(handle);