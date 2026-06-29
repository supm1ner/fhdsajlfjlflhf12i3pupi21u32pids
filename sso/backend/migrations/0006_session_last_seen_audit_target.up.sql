-- 0006_session_last_seen_audit_target.up.sql — deferred production-readiness
-- hardening (change: harden-and-observe).
--
-- 1) sessions.last_seen_at: records when a session last authenticated a request.
--    It is bumped (best-effort, throttled to at most once per minute per session)
--    on session use so the account + admin "active sessions" views can show a
--    last-active time. Defaults to now() so existing sessions have a sane value
--    immediately after the migration (and the column is NOT NULL).
--
-- 2) audit_log (target_type, target_id) index: backs the new audit target filter
--    (audit.Filter.TargetType/TargetID + the Journal API targetType/targetId
--    params), so the admin user-detail "recent activity" query selects an entity's
--    entries directly instead of scanning a recent 200-row window.

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS audit_log_target_idx ON audit_log (target_type, target_id);
