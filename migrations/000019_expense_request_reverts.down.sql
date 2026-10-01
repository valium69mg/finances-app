ALTER TABLE expense_requests
    DROP CONSTRAINT expense_requests_revert_audit,
    DROP COLUMN revert_count,
    DROP COLUMN reverted_at;
