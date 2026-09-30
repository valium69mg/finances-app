-- Gasto and Ingreso amounts must be positive; only Ahorro may be negative
-- (withdrawals). Gasto is already covered by the CHECK of 000006.
ALTER TABLE movements
    ADD CONSTRAINT movements_income_positive CHECK (kind <> 'Ingreso' OR amount > 0);
