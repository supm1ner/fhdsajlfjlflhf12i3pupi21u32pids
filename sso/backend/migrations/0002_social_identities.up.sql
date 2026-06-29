-- 0002_social_identities.up.sql — social-login support (change 3).
-- Adds the social_identities table mapping an external provider's (provider,
-- subject) pair to a cotton-id user, and a nullable avatar_url on users so a
-- provider-supplied profile picture can be stored.

CREATE TABLE IF NOT EXISTS social_identities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,                 -- google|github|vk|yandex
    provider_subject TEXT NOT NULL,                 -- the provider's stable user id
    email            CITEXT,                        -- email the provider asserted (may be unverified)
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_subject)
);

CREATE INDEX IF NOT EXISTS social_identities_user_id_idx ON social_identities (user_id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;
