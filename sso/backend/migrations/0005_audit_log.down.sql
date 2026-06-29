-- 0005_audit_log.down.sql — reverse of 0005_audit_log.up.sql.

DROP TABLE IF EXISTS audit_log;             -- drops the table and its triggers
DROP FUNCTION IF EXISTS audit_log_append_only();
