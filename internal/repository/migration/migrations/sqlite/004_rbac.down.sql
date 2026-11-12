-- +kilhog dialect: sqlite
-- SQLite cannot drop added columns, so groups_claim and oidc_groups remain.

DROP TABLE IF EXISTS grants;
