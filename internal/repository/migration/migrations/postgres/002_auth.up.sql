-- +kilhog dialect: postgres

CREATE TABLE IF NOT EXISTS local_users (
    uuid UUID PRIMARY KEY,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    display_name TEXT,
    email TEXT,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT local_users_username_unique UNIQUE (username)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_local_users_username_lower ON local_users (lower(username));

CREATE TABLE IF NOT EXISTS oidc_identity_pools (
    uuid UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    issuer TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    client_secret TEXT,
    scopes TEXT NOT NULL DEFAULT '["openid","profile","email"]',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    uuid UUID PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    principal_kind TEXT NOT NULL CHECK (principal_kind IN ('local_user', 'oidc')),
    local_user_uuid UUID REFERENCES local_users(uuid) ON DELETE CASCADE,
    identity_pool_uuid UUID REFERENCES oidc_identity_pools(uuid) ON DELETE SET NULL,
    oidc_subject TEXT,
    oidc_email TEXT,
    oidc_name TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_local_user ON sessions(local_user_uuid);

CREATE TABLE IF NOT EXISTS oidc_login_states (
    state TEXT PRIMARY KEY,
    pool_uuid UUID NOT NULL REFERENCES oidc_identity_pools(uuid) ON DELETE CASCADE,
    code_verifier TEXT NOT NULL,
    nonce TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_oidc_login_states_expires_at ON oidc_login_states(expires_at);
