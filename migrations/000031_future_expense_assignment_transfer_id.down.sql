-- Unlinks the future expense assignment pairs (a transfer_id group whose rows are
-- all "Asignado a ..." of Gastos futuros and include one linked to an item).
-- Note: this also unlinks assignments created after the up migration.
UPDATE movements
SET transfer_id = NULL
WHERE transfer_id IN (
    SELECT transfer_id FROM movements
    WHERE transfer_id IS NOT NULL
    GROUP BY transfer_id
    HAVING bool_and(category = 'Gastos futuros' AND description LIKE 'Asignado a %')
       AND bool_or(future_expense_id IS NOT NULL));

DROP INDEX IF EXISTS personal_movements_transfer_id_idx;
ALTER TABLE personal_movements DROP COLUMN IF EXISTS transfer_id;
