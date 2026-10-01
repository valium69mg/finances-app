-- Household step 3 follow-up: the owner can revert an approved expense request
-- back to 'solicitada'. The audit trail is two columns on the request itself:
-- how many times it was reverted and when it last was. The CHECK that ties the
-- decision columns to the status is unchanged: a reverted request is a plain
-- pending one (no decision, no result link).
ALTER TABLE expense_requests
    ADD COLUMN reverted_at  timestamptz,
    ADD COLUMN revert_count integer NOT NULL DEFAULT 0 CHECK (revert_count >= 0),
    ADD CONSTRAINT expense_requests_revert_audit CHECK ((revert_count = 0) = (reverted_at IS NULL));
