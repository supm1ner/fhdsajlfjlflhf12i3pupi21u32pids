-- 0004_account_self_service.down.sql — reverse of 0004_account_self_service.up.sql.

DROP TABLE IF EXISTS profile_images;

ALTER TABLE users
    DROP COLUMN IF EXISTS login_notifications,
    DROP COLUMN IF EXISTS pref_lang,
    DROP COLUMN IF EXISTS pref_theme,
    DROP COLUMN IF EXISTS banner_url;
