-- 0004_account_self_service.up.sql — account self-service support
-- (change: add-account-self-service).
--
-- Adds the banner image URL and server-side preference columns to users (so
-- theme/language/login-notification settings sync across a user's devices), and
-- a profile_images blob store holding the user's avatar and banner bytes. Images
-- are small, bounded blobs (type/size validated by the app) served from an
-- auth-gated account route; no object store / CDN is needed at this scale
-- (design.md D2/D6).

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS banner_url          TEXT,
    ADD COLUMN IF NOT EXISTS pref_theme          TEXT    NOT NULL DEFAULT 'system',
    ADD COLUMN IF NOT EXISTS pref_lang           TEXT    NOT NULL DEFAULT 'ru',
    ADD COLUMN IF NOT EXISTS login_notifications BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS profile_images (
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,                       -- avatar|banner
    content_type TEXT NOT NULL,                       -- image/png|image/jpeg|image/webp
    bytes        BYTEA NOT NULL,                      -- the raw image data
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind)
);
