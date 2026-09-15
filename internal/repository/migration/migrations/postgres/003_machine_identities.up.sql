-- +kilhog dialect: postgres

CREATE TABLE IF NOT EXISTS machine_pools (
    uuid UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS machine_identity_providers (
    uuid UUID PRIMARY KEY,
    machine_pool_uuid UUID NOT NULL REFERENCES machine_pools(uuid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    issuer TEXT NOT NULL,
    audiences TEXT NOT NULL,
    jwks_mode TEXT NOT NULL CHECK (jwks_mode IN ('discovery', 'uri', 'static')),
    jwks_uri TEXT,
    jwks TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (machine_pool_uuid, name),
    UNIQUE (machine_pool_uuid, issuer)
);

CREATE INDEX IF NOT EXISTS idx_machine_identity_providers_issuer
    ON machine_identity_providers(issuer);

CREATE TABLE IF NOT EXISTS machines (
    uuid UUID PRIMARY KEY,
    machine_pool_uuid UUID NOT NULL REFERENCES machine_pools(uuid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    provider_uuid UUID REFERENCES machine_identity_providers(uuid) ON DELETE SET NULL,
    subject TEXT,
    subject_prefix TEXT,
    claims TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (machine_pool_uuid, name)
);

CREATE INDEX IF NOT EXISTS idx_machines_pool ON machines(machine_pool_uuid);
CREATE INDEX IF NOT EXISTS idx_machines_provider ON machines(provider_uuid);

CREATE TABLE IF NOT EXISTS machine_api_keys (
    uuid UUID PRIMARY KEY,
    machine_uuid UUID NOT NULL REFERENCES machines(uuid) ON DELETE CASCADE,
    name TEXT,
    prefix TEXT NOT NULL UNIQUE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_machine_api_keys_machine ON machine_api_keys(machine_uuid);
CREATE INDEX IF NOT EXISTS idx_machine_api_keys_prefix ON machine_api_keys(prefix);
