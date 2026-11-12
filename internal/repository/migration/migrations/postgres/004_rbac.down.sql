-- +kilhog dialect: postgres

DROP TABLE IF EXISTS grants;
ALTER TABLE sessions DROP COLUMN IF EXISTS oidc_groups;
ALTER TABLE oidc_identity_pools DROP COLUMN IF EXISTS groups_claim;
