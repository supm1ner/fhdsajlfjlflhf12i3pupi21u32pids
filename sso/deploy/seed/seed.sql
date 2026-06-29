-- ─────────────────────────────────────────────────────────────────────────────
-- cotton-id DEV SEED — demo admin user (dev profile only; NEVER run in prod)
--
-- Inserts one demo account into the cotton-id `users` table so a developer can
-- log in immediately after `make up`. The schema is created by the backend's
-- embedded migration 0001 on boot, so this seed MUST run AFTER the backend has
-- migrated (the seed one-shot in compose waits on backend health).
--
--   Login:    admin@cotton.local
--   Username: admin
--   Password: DemoAdmin!2026
--
-- The password_hash below is a real argon2id PHC string generated with the
-- contract §5 parameters (m=64MiB, t=3, p=4, 16-byte salt, 32-byte key). It is a
-- well-known DEV credential, intentionally committed; production must create
-- real users through signup and must never load this file.
--
-- Idempotent: ON CONFLICT DO NOTHING keys on the unique email/username so
-- re-running the seed is a no-op.
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO users (
    email,
    email_verified,
    username,
    display_name,
    password_hash,
    status,
    role
) VALUES (
    'admin@cotton.local',
    TRUE,
    'admin',
    'Demo Admin',
    '$argon2id$v=19$m=65536,t=3,p=4$1fAoJrmrj/N0c544j43Fng$p152cspizWuKAqcQNOLeMpuI8ub/Sli7wFo5z+Fod+o',
    'active',
    'admin'
)
ON CONFLICT (email) DO NOTHING;
