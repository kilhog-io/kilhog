-- +kilhog dialect: sqlite

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS networks (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS subnets (
    uuid TEXT PRIMARY KEY,
    network_uuid TEXT NOT NULL REFERENCES networks(uuid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    prefix INTEGER NOT NULL,
    address TEXT NOT NULL,
    address_type TEXT NOT NULL CHECK (address_type IN ('ipv4', 'ipv6')),
    parent_kind TEXT NOT NULL CHECK (parent_kind IN ('network', 'subnet')),
    parent_uuid TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (network_uuid, name)
);

CREATE INDEX IF NOT EXISTS idx_subnets_network_uuid ON subnets(network_uuid);
CREATE INDEX IF NOT EXISTS idx_subnets_parent ON subnets(parent_kind, parent_uuid);

CREATE TABLE IF NOT EXISTS tags (
    resource_kind TEXT NOT NULL CHECK (resource_kind IN ('network', 'subnet')),
    resource_uuid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (resource_kind, resource_uuid, key)
);
