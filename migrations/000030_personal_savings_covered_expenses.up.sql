-- Same rule as 000028 for the household ledger: an expense paid from the
-- savings of a personal future expense must not distort her monthly KPIs.
--
--  * personal_movements.covered_by_savings: on the Gasto row, the part of the
--    payment covered by the savings of the item (min(saved, paid)). NULL means
--    none.
--  * personal_movements.savings_settlement: on the Ahorro rows a payment
--    creates (the release row and the remainder row, whose net is
--    -covered_by_savings).
--
-- Her savings totals, the free balance and the item totals keep reading every
-- Ahorro row; only the monthly KPIs ignore the marked amounts.
ALTER TABLE personal_movements
    ADD COLUMN covered_by_savings numeric(14,2),
    ADD COLUMN savings_settlement boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT personal_movements_covered_by_savings_expense_only
        CHECK (covered_by_savings IS NULL OR (kind = 'Gasto' AND covered_by_savings > 0)),
    ADD CONSTRAINT personal_movements_savings_settlement_savings_only
        CHECK (NOT savings_settlement OR kind = 'Ahorro');

-- Backfill the items already paid. The release row is linked to the item (the
-- payment wrote "Pago de <name>" on the date of the expense); the remainder row
-- is the unlinked "Sobrante de <name>" of the same date and user. The payment
-- trims the description to 120 characters, so the match does too. A payment
-- with nothing saved wrote neither and stays unmarked.
UPDATE personal_movements r
SET savings_settlement = true
FROM personal_future_expenses f
JOIN personal_movements e ON e.id = f.expense_movement_id AND e.user_id = f.user_id
WHERE f.status = 'paid'
  AND r.user_id = f.user_id
  AND r.kind = 'Ahorro'
  AND r.date = e.date
  AND r.category_id IN (SELECT id FROM personal_categories WHERE user_id = f.user_id AND kind = 'Ahorro' AND name = 'Gastos futuros')
  AND (
        (r.future_expense_id = f.id AND r.amount < 0 AND r.description = btrim(left('Pago de ' || f.name, 120)))
     OR (r.future_expense_id IS NULL AND r.amount > 0 AND r.description = btrim(left('Sobrante de ' || f.name, 120)))
  );

-- covered = what the release rows took out, never more than the expense.
UPDATE personal_movements e
SET covered_by_savings = LEAST(c.covered, e.amount)
FROM (
    SELECT f.expense_movement_id AS id, f.user_id, -SUM(r.amount) AS covered
    FROM personal_future_expenses f
    JOIN personal_movements r ON r.future_expense_id = f.id AND r.user_id = f.user_id
                              AND r.savings_settlement AND r.amount < 0
    WHERE f.status = 'paid' AND f.expense_movement_id IS NOT NULL
    GROUP BY f.expense_movement_id, f.user_id
) c
WHERE e.id = c.id AND e.user_id = c.user_id AND e.kind = 'Gasto' AND c.covered > 0;
