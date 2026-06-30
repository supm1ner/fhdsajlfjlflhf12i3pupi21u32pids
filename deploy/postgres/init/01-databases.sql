-- Runs once on first init of an empty Postgres volume, against POSTGRES_DB (cottonid),
-- as the superuser. Creates the Hydra and Sunrise databases and the extensions cotton-id needs.

CREATE DATABASE hydra;
CREATE DATABASE sunrise;

-- cotton-id (cottonid DB) uses citext + pgcrypto.
\connect cottonid
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
