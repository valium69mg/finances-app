ALTER TABLE personal_movements
    DROP CONSTRAINT IF EXISTS personal_movements_savings_settlement_savings_only,
    DROP CONSTRAINT IF EXISTS personal_movements_covered_by_savings_expense_only,
    DROP COLUMN IF EXISTS savings_settlement,
    DROP COLUMN IF EXISTS covered_by_savings;
