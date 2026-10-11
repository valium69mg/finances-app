DROP INDEX IF EXISTS personal_movements_future_expense_idx;
ALTER TABLE personal_movements
    DROP CONSTRAINT IF EXISTS personal_movements_future_expense_fk,
    DROP CONSTRAINT IF EXISTS personal_movements_future_expense_savings_only,
    DROP COLUMN IF EXISTS future_expense_id;
DROP TABLE IF EXISTS personal_future_expenses;
ALTER TABLE personal_movements DROP CONSTRAINT IF EXISTS personal_movements_id_user;
-- The seeded 'Gastos futuros' category is left in place: it is harmless and
-- movements may reference it.
