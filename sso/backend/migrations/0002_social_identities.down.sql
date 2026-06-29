-- 0002_social_identities.down.sql — reverse of 0002_social_identities.up.sql.

ALTER TABLE users DROP COLUMN IF EXISTS avatar_url;

DROP TABLE IF EXISTS social_identities;
