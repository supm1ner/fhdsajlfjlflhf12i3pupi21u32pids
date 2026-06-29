-- ─────────────────────────────────────────────────────────────────────────────
-- Postgres bootstrap for cotton-id (runs once, on an EMPTY data volume).
--
-- The official postgres image executes every *.sql / *.sh in
-- /docker-entrypoint-initdb.d in lexical order, against the database named by
-- POSTGRES_DB, the first time the data directory is initialized. We use it to:
--   1. create the SEPARATE `hydra` database (contract §4 — Hydra uses its own DB
--      in the same instance), and
--   2. make the citext + pgcrypto extensions available in the cotton-id DB so
--      migration 0001 (CITEXT columns, gen_random_uuid()) can run.
--
-- Idempotency note: this script ONLY runs against a fresh volume. To re-run it,
-- remove the `pgdata` named volume (`make down` + volume prune, or compose down
-- -v). The extension CREATE statements use IF NOT EXISTS so re-applying the
-- file by hand is also safe.
-- ─────────────────────────────────────────────────────────────────────────────

-- 1) Hydra's own database. Hydra migrates it via `hydra migrate sql` (the
--    hydra-migrate one-shot in compose). Owned by the same app role for dev
--    simplicity; split roles in production if desired.
SELECT 'CREATE DATABASE hydra OWNER cotton'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'hydra')\gexec

-- 2) Extensions in the cotton-id application database (POSTGRES_DB = cottonid).
--    Migration 0001 also issues CREATE EXTENSION IF NOT EXISTS as a belt-and-
--    suspenders measure, but creating them here guarantees availability even if
--    the app role lacks CREATE EXTENSION privilege in a hardened setup.
\connect cottonid
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
