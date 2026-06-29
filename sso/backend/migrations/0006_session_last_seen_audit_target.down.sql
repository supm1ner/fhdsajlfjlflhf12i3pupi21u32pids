-- 0006_session_last_seen_audit_target.down.sql — reverse of the up migration.

DROP INDEX IF EXISTS audit_log_target_idx;

ALTER TABLE sessions
    DROP COLUMN IF EXISTS last_seen_at;
