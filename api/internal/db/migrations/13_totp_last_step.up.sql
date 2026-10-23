-- The time step of the last TOTP code accepted for the user. A code is only
-- accepted when its step is newer, so each code works once.
ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0;
