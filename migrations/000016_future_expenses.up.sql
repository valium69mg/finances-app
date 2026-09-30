-- Planned future expenses ("Gastos futuros"): things the owner chooses to save
-- for in advance. Amounts are MXN only. A paid item keeps the amount actually
-- paid and the Gasto movement registered for it (NULL if that expense is
-- deleted later). Nothing is hard-wired to the bills module: a yearly bill is
-- not necessarily something the owner saves for.
CREATE TABLE future_expenses (
    id                  bigserial     PRIMARY KEY,
    name                text          NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 120),
    target_amount       numeric(14,2) NOT NULL CHECK (target_amount > 0),
    due_date            date          NOT NULL,
    status              text          NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paid')),
    paid_at             date,
    amount_paid         numeric(14,2) CHECK (amount_paid IS NULL OR amount_paid > 0),
    expense_movement_id bigint        REFERENCES movements (id) ON DELETE SET NULL,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    updated_at          timestamptz   NOT NULL DEFAULT now(),
    CHECK (
        (status = 'active' AND paid_at IS NULL AND amount_paid IS NULL AND expense_movement_id IS NULL)
        OR (status = 'paid' AND paid_at IS NOT NULL AND amount_paid IS NOT NULL)
    )
);

CREATE INDEX future_expenses_status_due_idx ON future_expenses (status, due_date);

-- Ahorro rows that feed an item (contributions, assignments from the free
-- balance and the release row of a payment) carry its id. Deleting an item
-- unlinks them (they return to the free balance) instead of deleting money.
ALTER TABLE movements
    ADD COLUMN future_expense_id bigint REFERENCES future_expenses (id) ON DELETE SET NULL,
    ADD CONSTRAINT movements_future_expense_savings_only CHECK (future_expense_id IS NULL OR kind = 'Ahorro');

CREATE INDEX movements_future_expense_id_idx ON movements (future_expense_id) WHERE future_expense_id IS NOT NULL;
