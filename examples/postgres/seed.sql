-- RDAP Server — PostgreSQL example seed data
-- Five example domains with registrant/registrar/technical entities, nameservers,
-- an IP network and an autnum. Load after schema.sql.
--
-- Handles use the <local>-<EPPROID> ROID form; "NAME" is a repository ID
-- registered in the IANA EPP Repository Identifiers registry.

-- ---------------------------------------------------------------------------
-- Entities
-- ---------------------------------------------------------------------------
INSERT INTO entities (handle, vcard_json, roles, status, public_ids) VALUES
    -- IANA Registrar ID 2 (Network Solutions sample)
    (
        '2',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","Example Registrar Inc."],["adr",{"cc":"US"},"text",["","","123 Maple Ave","Los Angeles","CA","90210",""]]]]',
        '["registrar"]',
        '["active"]',
        '[{"type":"IANA Registrar ID","identifier":"2"}]'
    ),
    -- Technical contact
    (
        '888',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","Example Technical Contact"],["email",{},"text","tech@example.com"]]]',
        '["technical"]',
        '["active"]',
        '[]'
    ),
    -- Registrants
    (
        'REG1-NAME',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","Alice Anderson"],["org",{},"text","Anderson LLC"],["adr",{"cc":"US"},"text",["","","123 Elm Street","Springfield","IL","62701",""]],["tel",{"type":["voice"]},"uri","tel:+1-217-555-0132"],["email",{},"text","alice@example.com"]]]',
        '["registrant"]',
        '["active"]',
        '[]'
    ),
    (
        'REG2-NAME',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","Bob Baker"],["org",{},"text","Baker Ltd"],["adr",{"cc":"GB"},"text",["","","9 Baker Street","London","","NW1 6XE",""]],["tel",{"type":["voice"]},"uri","tel:+44-20-7946-0958"],["email",{},"text","bob@example.net"]]]',
        '["registrant"]',
        '["active"]',
        '[]'
    ),
    (
        'REG3-NAME',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","Carol Chen"],["org",{},"text","Chen & Co"],["adr",{"cc":"CA"},"text",["","","45 Yonge Street","Toronto","ON","M5E 1G5",""]],["tel",{"type":["voice"]},"uri","tel:+1-416-555-0142"],["email",{},"text","carol@example.org"]]]',
        '["registrant"]',
        '["active"]',
        '[]'
    ),
    (
        'REG4-NAME',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","David Deng"],["org",{},"text","Deng GmbH"],["adr",{"cc":"DE"},"text",["","","1 Unter den Linden","Berlin","","10117",""]],["tel",{"type":["voice"]},"uri","tel:+49-30-901820"],["email",{},"text","david@buecher.example"]]]',
        '["registrant"]',
        '["active"]',
        '[]'
    ),
    (
        'REG5-NAME',
        '["vcard",[["version",{},"text","4.0"],["fn",{},"text","Eve Evans"],["org",{},"text","Evans Pty"],["adr",{"cc":"AU"},"text",["","","77 George Street","Sydney","NSW","2000",""]],["tel",{"type":["voice"]},"uri","tel:+61-2-5550-0147"],["email",{},"text","eve@example.info"]]]',
        '["registrant"]',
        '["active"]',
        '[]'
    )
ON CONFLICT (handle) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Nameservers
-- ---------------------------------------------------------------------------
INSERT INTO nameservers (handle, ldh_name, unicode_name, ipv4, ipv6, status) VALUES
    ('NS1-NAME', 'ns1.example.com', 'ns1.example.com', '["8.8.8.8"]', '["2001:4860:4860::8888"]', '["associated"]'),
    ('NS2-NAME', 'ns2.example.com', 'ns2.example.com', '["1.1.1.1"]', '["2606:4700:4700::1111"]', '["associated"]'),
    ('NS3-NAME', 'ns3.example.net', 'ns3.example.net', '["8.8.4.4"]', '["2001:4860:4860::8844"]', '["associated"]'),
    ('NS4-NAME', 'ns4.example.net', 'ns4.example.net', '["1.0.0.1"]', '["2606:4700:4700::1001"]', '["associated"]')
ON CONFLICT (handle) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Domains
-- ---------------------------------------------------------------------------
INSERT INTO domains (handle, ldh_name, unicode_name, tld, status, expires_at, registrant, admin, tech, nameservers, secure_dns) VALUES
    (
        'EX1-NAME', 'example.com', 'example.com', 'com', '["associated"]',
        NOW() + INTERVAL '1 year', 'REG1-NAME', '888', '888',
        '[{"handle":"NS1-NAME","ldhName":"ns1.example.com","unicodeName":"ns1.example.com","ipv4":["8.8.8.8"],"ipv6":["2001:4860:4860::8888"],"status":["associated"]},{"handle":"NS2-NAME","ldhName":"ns2.example.com","unicodeName":"ns2.example.com","ipv4":["1.1.1.1"],"ipv6":["2606:4700:4700::1111"],"status":["associated"]}]',
        '{"zoneSigned":false,"delegationSigned":false}'
    ),
    (
        'EX2-NAME', 'example.net', 'example.net', 'net', '["associated"]',
        NOW() + INTERVAL '2 years', 'REG2-NAME', '888', '888',
        '[{"handle":"NS3-NAME","ldhName":"ns3.example.net","unicodeName":"ns3.example.net","ipv4":["8.8.4.4"],"ipv6":["2001:4860:4860::8844"],"status":["associated"]},{"handle":"NS4-NAME","ldhName":"ns4.example.net","unicodeName":"ns4.example.net","ipv4":["1.0.0.1"],"ipv6":["2606:4700:4700::1001"],"status":["associated"]}]',
        '{"zoneSigned":true,"delegationSigned":true,"maxSigLife":86400,"dsData":[{"keyTag":12345,"algorithm":13,"digestType":2,"digest":"6FD88D0A1C9E4B8B7E9A8C9B1A2B3C4D5E6F7A8B9C0D1E2F3A4B5C6D7E8F9A0"}]}'
    ),
    (
        'EX3-NAME', 'example.org', 'example.org', 'org', '["associated"]',
        NOW() + INTERVAL '3 years', 'REG3-NAME', '888', '888',
        '[{"handle":"NS1-NAME","ldhName":"ns1.example.com","unicodeName":"ns1.example.com","ipv4":["8.8.8.8"],"ipv6":["2001:4860:4860::8888"],"status":["associated"]},{"handle":"NS3-NAME","ldhName":"ns3.example.net","unicodeName":"ns3.example.net","ipv4":["8.8.4.4"],"ipv6":["2001:4860:4860::8844"],"status":["associated"]}]',
        '{"zoneSigned":false,"delegationSigned":false}'
    ),
    (
        'EX4-NAME', 'xn--bcher-kva.com', 'bücher.com', 'com', '["associated"]',
        NOW() + INTERVAL '1 year', 'REG4-NAME', '888', '888',
        '[{"handle":"NS1-NAME","ldhName":"ns1.example.com","unicodeName":"ns1.example.com","ipv4":["8.8.8.8"],"ipv6":["2001:4860:4860::8888"],"status":["associated"]},{"handle":"NS2-NAME","ldhName":"ns2.example.com","unicodeName":"ns2.example.com","ipv4":["1.1.1.1"],"ipv6":["2606:4700:4700::1111"],"status":["associated"]}]',
        '{"zoneSigned":false,"delegationSigned":false}'
    ),
    (
        'EX5-NAME', 'example.info', 'example.info', 'info', '["associated"]',
        NOW() + INTERVAL '1 year', 'REG5-NAME', '888', '888',
        '[{"handle":"NS2-NAME","ldhName":"ns2.example.com","unicodeName":"ns2.example.com","ipv4":["1.1.1.1"],"ipv6":["2606:4700:4700::1111"],"status":["associated"]},{"handle":"NS4-NAME","ldhName":"ns4.example.net","unicodeName":"ns4.example.net","ipv4":["1.0.0.1"],"ipv6":["2606:4700:4700::1001"],"status":["associated"]}]',
        '{"zoneSigned":false,"delegationSigned":false}'
    )
ON CONFLICT (handle) DO NOTHING;

INSERT INTO domain_nameservers (domain_handle, ns_handle) VALUES
    ('EX1-NAME', 'NS1-NAME'),
    ('EX1-NAME', 'NS2-NAME'),
    ('EX2-NAME', 'NS3-NAME'),
    ('EX2-NAME', 'NS4-NAME'),
    ('EX3-NAME', 'NS1-NAME'),
    ('EX3-NAME', 'NS3-NAME'),
    ('EX4-NAME', 'NS1-NAME'),
    ('EX4-NAME', 'NS2-NAME'),
    ('EX5-NAME', 'NS2-NAME'),
    ('EX5-NAME', 'NS4-NAME')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- IP networks
-- ---------------------------------------------------------------------------
INSERT INTO ip_networks (handle, start_address, end_address, ip_version, cidr, name, type, country, status) VALUES
    ('NET-8-8-8-0-24',  '8.8.8.0',   '8.8.8.255',   'v4', ARRAY['8.8.8.0/24'],          'GOOGLE',   'ALLOCATED', 'US', '["active"]'),
    ('NET-1-0-0-0-24',  '1.0.0.0',   '1.0.0.255',   'v4', ARRAY['1.0.0.0/24'],          'CLOUDFLARE','ALLOCATED', 'AU', '["active"]'),
    ('NET-2001-4860-0-32', '2001:4860::', '2001:4860::ffff:ffff', 'v6', ARRAY['2001:4860::/32'], 'GOOGLE', 'ALLOCATED', 'US', '["active"]')
ON CONFLICT (handle) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Autonomous systems
-- ---------------------------------------------------------------------------
INSERT INTO autnums (handle, start_asn, end_asn, name, type, country, status) VALUES
    ('AS15169', 15169, 15169, 'GOOGLE', 'DIRECT ALLOCATION', 'US', '["active"]'),
    ('AS13335', 13335, 13335, 'CLOUDFLARE', 'DIRECT ALLOCATION', 'US', '["active"]')
ON CONFLICT (handle) DO NOTHING;