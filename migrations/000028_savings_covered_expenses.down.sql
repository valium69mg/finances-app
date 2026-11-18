ALTER TABLE movements
    DROP CONSTRAINT IF EXISTS movements_savings_settlement_savings_only,
    DROP CONSTRAINT IF EXISTS movements_covered_by_savings_expense_only,
    DROP COLUMN IF EXISTS savings_settlement,
    DROP COLUMN IF EXISTS covered_by_savings;
