DROP INDEX IF EXISTS verification_tokens_user_purpose_idx;
-- Reset tokens have no meaning without the column; drop them so a rollback cannot
-- turn them into verification tokens.
DELETE FROM verification_tokens WHERE purpose = 'reset';
ALTER TABLE verification_tokens DROP COLUMN purpose;
