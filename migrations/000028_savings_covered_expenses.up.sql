-- An expense paid from the savings of a future expense must not distort the
-- monthly KPIs: the money was set aside in earlier months and the savings side
-- of the payment is only a settlement of that reserve.
--
--  * movements.covered_by_savings: on the Gasto row, the part of the payment
--    covered by the savings of the item (min(saved, paid)). NULL means none.
--  * movements.savings_settlement: on the Ahorro rows a payment creates (the
--    release row and the remainder row, whose net is -covered_by_savings).
--
-- Both are explicit markers so no reader has to guess from the links; the
-- savings portfolio, the emergency fund, the free balance and the item totals
-- keep reading every Ahorro row as before. Only monthly KPIs ignore the marked
-- amounts.
ALTER TABLE movements
    ADD COLUMN covered_by_savings numeric,
    ADD COLUMN savings_settlement boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT movements_covered_by_savings_expense_only
        CHECK (covered_by_savings IS NULL OR (kind = 'Gasto' AND covered_by_savings > 0)),
    ADD CONSTRAINT movements_savings_settlement_savings_only
        CHECK (NOT savings_settlement OR kind = 'Ahorro');

-- Backfill the items already paid. The release row is linked to the item (the
-- payment wrote "Pago de <name>" on the date of the expense); the remainder row
-- is the unlinked "Sobrante de <name>" of the same date. A payment with nothing
-- saved wrote neither and stays unmarked.
UPDATE movements r
SET savings_settlement = true
FROM future_expenses f
JOIN movements e ON e.id = f.expense_movement_id
WHERE f.status = 'paid'
  AND r.kind = 'Ahorro'
  AND r.date = e.date
  AND r.category = 'Gastos futuros'
  AND (
        (r.future_expense_id = f.id AND r.amount_mxn < 0 AND r.description = 'Pago de ' || f.name)
     OR (r.future_expense_id IS NULL AND r.amount_mxn > 0 AND r.description = 'Sobrante de ' || f.name)
  );

-- covered = what the release rows took out, never more than the expense.
UPDATE movements e
SET covered_by_savings = LEAST(c.covered, e.amount_mxn)
FROM (
    SELECT f.expense_movement_id AS id, -SUM(r.amount_mxn) AS covered
    FROM future_expenses f
    JOIN movements r ON r.future_expense_id = f.id AND r.savings_settlement AND r.amount_mxn < 0
    WHERE f.status = 'paid' AND f.expense_movement_id IS NOT NULL
    GROUP BY f.expense_movement_id
) c
WHERE e.id = c.id AND e.kind = 'Gasto' AND c.covered > 0;
