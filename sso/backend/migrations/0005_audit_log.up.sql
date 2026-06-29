-- 0005_audit_log.up.sql — persistent audit log (change: add-admin-console).
--
-- Adds the audit_log table backing the admin console's Journal. The whole
-- backend appends security-relevant and administrative events here (login
-- ok/fail, signup, password reset, OIDC consent, OAuth client registration, and
-- admin lifecycle actions), in addition to the existing structured slog lines.
-- The table is append-only via the application (no UPDATE/DELETE path) so it is a
-- durable, queryable trail (design.md D2). Writes are best-effort: a failed
-- insert is logged at error and never blocks the user action.
--
-- actor_id is nullable (failed logins, unauthenticated events have no resolved
-- actor); actor_label carries a human-readable actor for display even when the
-- id is absent. metadata is free-form JSONB for action-specific context.

CREATE TABLE IF NOT EXISTS audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id    UUID,                       -- the acting user, when resolved (nullable)
    actor_label TEXT,                       -- human-readable actor (email/username/"anonymous")
    action      TEXT NOT NULL,              -- e.g. auth.login.ok, admin.user.suspend
    target_type TEXT,                       -- e.g. user, client, session
    target_id   TEXT,                       -- the affected entity's id (text: ids are heterogeneous)
    ip          TEXT,                       -- request client IP (trusted-proxy-aware)
    request_id  TEXT,                       -- correlation id (X-Request-Id)
    metadata    JSONB                       -- action-specific context
);

CREATE INDEX IF NOT EXISTS audit_log_ts_idx ON audit_log (ts DESC);
CREATE INDEX IF NOT EXISTS audit_log_actor_id_idx ON audit_log (actor_id);
CREATE INDEX IF NOT EXISTS audit_log_action_idx ON audit_log (action);

-- Enforce append-only at the DATABASE level, not just by application convention:
-- reject any UPDATE / DELETE / TRUNCATE on audit_log so the trail cannot be
-- tampered with or erased even by the application role, a future code change, or
-- an SQL-injection elsewhere. INSERT and SELECT remain allowed. (Dropping the
-- table for a rollback still works — DROP is not UPDATE/DELETE/TRUNCATE.)
CREATE OR REPLACE FUNCTION audit_log_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_mutate_row
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();

CREATE TRIGGER audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();
