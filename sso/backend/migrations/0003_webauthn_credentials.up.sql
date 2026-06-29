-- 0003_webauthn_credentials.up.sql — passkey (WebAuthn/FIDO2) support (change: add-passkey-auth).
-- Stores one row per registered WebAuthn credential, bound to a cotton-id user.
-- The library (github.com/go-webauthn/webauthn) supplies the credential id, COSE
-- public key, attestation type, AAGUID, sign counter, and transports; cotton-id
-- adds a user-chosen nickname and audit timestamps. The sign_count is updated on
-- every successful assertion and a non-increasing value is treated as a possible
-- cloned authenticator (clone detection).

CREATE TABLE IF NOT EXISTS webauthn_credentials (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id    BYTEA NOT NULL UNIQUE,          -- raw credential ID (lookup key)
    public_key       BYTEA NOT NULL,                 -- COSE public key
    attestation_type TEXT,                           -- e.g. none|basic_full|...
    aaguid           BYTEA,                          -- authenticator model identifier
    sign_count       BIGINT NOT NULL DEFAULT 0,      -- last seen signature counter
    transports       TEXT[],                         -- e.g. {internal,hybrid,usb}
    name             TEXT NOT NULL DEFAULT '',       -- user-chosen nickname
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS webauthn_credentials_user_id_idx ON webauthn_credentials (user_id);
