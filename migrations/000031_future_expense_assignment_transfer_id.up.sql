-- A future expense assignment is two Ahorro rows of Gastos futuros: -X linked to
-- no item and +X linked to the item. They now share a transfer_id, like a
-- Traspaso pair, so they are listed as one row and deleted together. Nothing
-- that computes balances reads transfer_id, so no total changes.
--
-- personal_movements gets its own transfer_id (the spouse's module has the same
-- flow); movements already has the column since 000009.
ALTER TABLE personal_movements ADD COLUMN transfer_id uuid;
CREATE INDEX personal_movements_transfer_id_idx ON personal_movements (user_id, transfer_id) WHERE transfer_id IS NOT NULL;

-- Backfill the owner's existing pairs. Rows are paired per (date, description,
-- absolute amount) in id order, so duplicates match deterministically.
WITH out_rows AS (
    SELECT id, row_number() OVER (PARTITION BY date, description, abs(amount) ORDER BY id) AS rn,
           date, description, abs(amount) AS amt
    FROM movements
    WHERE kind = 'Ahorro' AND category = 'Gastos futuros' AND transfer_id IS NULL
      AND description LIKE 'Asignado a %' AND amount < 0 AND future_expense_id IS NULL
), in_rows AS (
    SELECT id, row_number() OVER (PARTITION BY date, description, amount ORDER BY id) AS rn,
           date, description, amount AS amt
    FROM movements
    WHERE kind = 'Ahorro' AND category = 'Gastos futuros' AND transfer_id IS NULL
      AND description LIKE 'Asignado a %' AND amount > 0 AND future_expense_id IS NOT NULL
), pairs AS MATERIALIZED (
    SELECT o.id AS out_id, i.id AS in_id, gen_random_uuid() AS tid
    FROM out_rows o
    JOIN in_rows i ON i.date = o.date AND i.description = o.description AND i.amt = o.amt AND i.rn = o.rn
)
UPDATE movements m SET transfer_id = p.tid
FROM pairs p
WHERE m.id IN (p.out_id, p.in_id);

-- Same backfill for the spouse, scoped by user_id and her category.
WITH out_rows AS (
    SELECT m.id, m.user_id, row_number() OVER (PARTITION BY m.user_id, m.date, m.description, abs(m.amount) ORDER BY m.id) AS rn,
           m.date, m.description, abs(m.amount) AS amt
    FROM personal_movements m JOIN personal_categories c ON c.id = m.category_id
    WHERE m.kind = 'Ahorro' AND c.name = 'Gastos futuros' AND m.transfer_id IS NULL
      AND m.description LIKE 'Asignado a %' AND m.amount < 0 AND m.future_expense_id IS NULL
), in_rows AS (
    SELECT m.id, m.user_id, row_number() OVER (PARTITION BY m.user_id, m.date, m.description, m.amount ORDER BY m.id) AS rn,
           m.date, m.description, m.amount AS amt
    FROM personal_movements m JOIN personal_categories c ON c.id = m.category_id
    WHERE m.kind = 'Ahorro' AND c.name = 'Gastos futuros' AND m.transfer_id IS NULL
      AND m.description LIKE 'Asignado a %' AND m.amount > 0 AND m.future_expense_id IS NOT NULL
), pairs AS MATERIALIZED (
    SELECT o.id AS out_id, i.id AS in_id, gen_random_uuid() AS tid
    FROM out_rows o
    JOIN in_rows i ON i.user_id = o.user_id AND i.date = o.date AND i.description = o.description
                  AND i.amt = o.amt AND i.rn = o.rn
)
UPDATE personal_movements m SET transfer_id = p.tid
FROM pairs p
WHERE m.id IN (p.out_id, p.in_id);
