DROP INDEX IF EXISTS movements_future_expense_id_idx;
ALTER TABLE movements DROP CONSTRAINT IF EXISTS movements_future_expense_savings_only;
ALTER TABLE movements DROP COLUMN IF EXISTS future_expense_id;
DROP TABLE IF EXISTS future_expenses;
