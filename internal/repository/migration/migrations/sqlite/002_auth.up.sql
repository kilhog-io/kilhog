-- +kilhog dialect: sqlite

CREATE TABLE IF NOT EXISTS local_users (
    uuid TEXT PRIMARY KEY,
    username TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT,
    email TEXT,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oidc_identity_pools (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    issuer TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    client_secret TEXT,
    scopes TEXT NOT NULL DEFAULT '["openid","profile","email"]',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    uuid TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    principal_kind TEXT NOT NULL CHECK (principal_kind IN ('local_user', 'oidc')),
    local_user_uuid TEXT REFERENCES local_users(uuid) ON DELETE CASCADE,
    identity_pool_uuid TEXT REFERENCES oidc_identity_pools(uuid) ON DELETE SET NULL,
    oidc_subject TEXT,
    oidc_email TEXT,
    oidc_name TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_local_user ON sessions(local_user_uuid);

CREATE TABLE IF NOT EXISTS oidc_login_states (
    state TEXT PRIMARY KEY,
    pool_uuid TEXT NOT NULL REFERENCES oidc_identity_pools(uuid) ON DELETE CASCADE,
    code_verifier TEXT NOT NULL,
    nonce TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_oidc_login_states_expires_at ON oidc_login_states(expires_at);
