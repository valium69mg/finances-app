-- Verification and password reset tokens share one table; the purpose keeps a
-- token from being redeemed for the other flow. Existing rows are verification tokens.
ALTER TABLE verification_tokens
    ADD COLUMN purpose text NOT NULL DEFAULT 'verification'
    CONSTRAINT verification_tokens_purpose_check CHECK (purpose IN ('verification', 'reset'));

CREATE INDEX verification_tokens_user_purpose_idx ON verification_tokens (user_id, purpose);
