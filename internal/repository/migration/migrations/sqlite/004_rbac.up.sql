-- +kilhog dialect: sqlite

ALTER TABLE oidc_identity_pools ADD COLUMN groups_claim TEXT NOT NULL DEFAULT 'groups';
ALTER TABLE sessions ADD COLUMN oidc_groups TEXT NOT NULL DEFAULT '[]';

CREATE TABLE IF NOT EXISTS grants (
    uuid TEXT PRIMARY KEY,
    principal_kind TEXT NOT NULL CHECK (principal_kind IN ('local_user', 'oidc', 'oidc_group', 'machine', 'machine_pool')),
    local_user_uuid TEXT REFERENCES local_users(uuid) ON DELETE CASCADE,
    identity_pool_uuid TEXT REFERENCES oidc_identity_pools(uuid) ON DELETE CASCADE,
    oidc_subject TEXT,
    oidc_group TEXT,
    machine_uuid TEXT REFERENCES machines(uuid) ON DELETE CASCADE,
    machine_pool_uuid TEXT REFERENCES machine_pools(uuid) ON DELETE CASCADE,
    resource_kind TEXT NOT NULL CHECK (resource_kind IN ('network', 'subnet', 'platform')),
    resource_uuid TEXT,
    network_uuid TEXT REFERENCES networks(uuid) ON DELETE CASCADE,
    capability TEXT CHECK (capability IS NULL OR capability = 'create_networks'),
    can_create INTEGER NOT NULL CHECK (can_create IN (0, 1)),
    can_read INTEGER NOT NULL CHECK (can_read IN (0, 1)),
    can_update INTEGER NOT NULL CHECK (can_update IN (0, 1)),
    can_delete INTEGER NOT NULL CHECK (can_delete IN (0, 1)),
    owner INTEGER NOT NULL CHECK (owner IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (
        (principal_kind = 'local_user' AND local_user_uuid IS NOT NULL AND identity_pool_uuid IS NULL AND oidc_subject IS NULL AND oidc_group IS NULL AND machine_uuid IS NULL AND machine_pool_uuid IS NULL)
        OR (principal_kind = 'oidc' AND identity_pool_uuid IS NOT NULL AND oidc_subject IS NOT NULL AND local_user_uuid IS NULL AND oidc_group IS NULL AND machine_uuid IS NULL AND machine_pool_uuid IS NULL)
        OR (principal_kind = 'oidc_group' AND identity_pool_uuid IS NOT NULL AND oidc_group IS NOT NULL AND local_user_uuid IS NULL AND oidc_subject IS NULL AND machine_uuid IS NULL AND machine_pool_uuid IS NULL)
        OR (principal_kind = 'machine' AND machine_uuid IS NOT NULL AND local_user_uuid IS NULL AND identity_pool_uuid IS NULL AND oidc_subject IS NULL AND oidc_group IS NULL AND machine_pool_uuid IS NULL)
        OR (principal_kind = 'machine_pool' AND machine_pool_uuid IS NOT NULL AND local_user_uuid IS NULL AND identity_pool_uuid IS NULL AND oidc_subject IS NULL AND oidc_group IS NULL AND machine_uuid IS NULL)
    ),
    CHECK (
        (resource_kind IN ('network', 'subnet') AND resource_uuid IS NOT NULL AND network_uuid IS NOT NULL AND capability IS NULL AND (can_create + can_read + can_update + can_delete + owner) >= 1)
        OR (resource_kind = 'platform' AND resource_uuid IS NULL AND network_uuid IS NULL AND capability = 'create_networks' AND owner = 0 AND can_create = 1)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_local_resource
    ON grants (local_user_uuid, resource_kind, ifnull(resource_uuid, ''), ifnull(capability, ''))
    WHERE principal_kind = 'local_user';

CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_oidc_resource
    ON grants (identity_pool_uuid, oidc_subject, resource_kind, ifnull(resource_uuid, ''), ifnull(capability, ''))
    WHERE principal_kind = 'oidc';

CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_oidc_group_resource
    ON grants (identity_pool_uuid, oidc_group, resource_kind, ifnull(resource_uuid, ''), ifnull(capability, ''))
    WHERE principal_kind = 'oidc_group';

CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_machine_resource
    ON grants (machine_uuid, resource_kind, ifnull(resource_uuid, ''), ifnull(capability, ''))
    WHERE principal_kind = 'machine';

CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_machine_pool_resource
    ON grants (machine_pool_uuid, resource_kind, ifnull(resource_uuid, ''), ifnull(capability, ''))
    WHERE principal_kind = 'machine_pool';

CREATE INDEX IF NOT EXISTS idx_grants_resource ON grants (resource_kind, resource_uuid);
CREATE INDEX IF NOT EXISTS idx_grants_network ON grants (network_uuid);
CREATE INDEX IF NOT EXISTS idx_grants_local_user ON grants (local_user_uuid);
CREATE INDEX IF NOT EXISTS idx_grants_oidc_principal ON grants (identity_pool_uuid, oidc_subject);
CREATE INDEX IF NOT EXISTS idx_grants_oidc_group ON grants (identity_pool_uuid, oidc_group);
CREATE INDEX IF NOT EXISTS idx_grants_machine ON grants (machine_uuid);
CREATE INDEX IF NOT EXISTS idx_grants_machine_pool ON grants (machine_pool_uuid);
