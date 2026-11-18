ALTER TABLE categories
    DROP CONSTRAINT IF EXISTS categories_fixed_expense_only,
    DROP COLUMN IF EXISTS fixed;
