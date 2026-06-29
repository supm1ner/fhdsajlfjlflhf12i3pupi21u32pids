-- 0001_init.down.sql — reverse of 0001_init.up.sql (local dev only).
-- Extensions are intentionally left in place: they may be shared and dropping
-- them is rarely desirable.

DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
