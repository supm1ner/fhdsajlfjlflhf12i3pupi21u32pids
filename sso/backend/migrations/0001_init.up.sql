-- 0001_init.up.sql — cotton-id initial schema.
-- Enables the citext (case-insensitive email/username) and pgcrypto
-- (gen_random_uuid) extensions, then creates the core tables exactly per the
-- build contract §4.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email          CITEXT UNIQUE NOT NULL,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    username       CITEXT UNIQUE NOT NULL,
    display_name   TEXT NOT NULL,
    password_hash  TEXT,                            -- nullable: future social-only accounts
    status         TEXT NOT NULL DEFAULT 'active',  -- active|invited|suspended
    role           TEXT NOT NULL DEFAULT 'user',    -- user|admin|owner (seam; admin UI later)
    about          TEXT NOT NULL DEFAULT '',
    location       TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,                   -- sha256(opaque token) hex
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    remember    BOOLEAN NOT NULL DEFAULT FALSE,
    user_agent  TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_hash  TEXT PRIMARY KEY,                   -- sha256(token) hex
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);
