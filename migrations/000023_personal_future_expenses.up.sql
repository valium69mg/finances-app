-- Phase 12, step 4: future expenses of a household user, same model as the
-- owner's (000016) in her own tables. MXN only. Nothing here touches movements
-- or future_expenses.

-- Target of the composite foreign key of expense_movement_id, so a payment can
-- only point at a movement of the same user.
ALTER TABLE personal_movements ADD CONSTRAINT personal_movements_id_user UNIQUE (id, user_id);

CREATE TABLE personal_future_expenses (
    id                  bigserial     PRIMARY KEY,
    user_id             uuid          NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name                text          NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 120),
    target_amount       numeric(14,2) NOT NULL CHECK (target_amount > 0),
    due_date            date          NOT NULL,
    status              text          NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paid')),
    paid_at             date,
    amount_paid         numeric(14,2) CHECK (amount_paid IS NULL OR amount_paid > 0),
    expense_movement_id bigint,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    updated_at          timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT personal_future_expenses_paid_consistent CHECK (
        (status = 'active' AND paid_at IS NULL AND amount_paid IS NULL AND expense_movement_id IS NULL)
        OR (status = 'paid' AND paid_at IS NOT NULL AND amount_paid IS NOT NULL)),
    -- Target of the composite foreign key of personal_movements.future_expense_id.
    CONSTRAINT personal_future_expenses_id_user UNIQUE (id, user_id),
    -- Deleting the Gasto only clears the link; user_id is never nulled.
    CONSTRAINT personal_future_expenses_expense_fk FOREIGN KEY (expense_movement_id, user_id)
        REFERENCES personal_movements (id, user_id) ON DELETE SET NULL (expense_movement_id)
);

CREATE INDEX personal_future_expenses_user_status_due ON personal_future_expenses (user_id, status, due_date);

-- Ahorro rows that feed an item (contributions, assignments from the free
-- balance and the release row of a payment) carry its id. Deleting an item
-- unlinks them (they return to the free balance) instead of deleting money;
-- only future_expense_id is nulled, user_id stays.
ALTER TABLE personal_movements
    ADD COLUMN future_expense_id bigint,
    ADD CONSTRAINT personal_movements_future_expense_savings_only CHECK (future_expense_id IS NULL OR kind = 'Ahorro'),
    ADD CONSTRAINT personal_movements_future_expense_fk FOREIGN KEY (future_expense_id, user_id)
        REFERENCES personal_future_expenses (id, user_id) ON DELETE SET NULL (future_expense_id);

CREATE INDEX personal_movements_future_expense_idx ON personal_movements (future_expense_id) WHERE future_expense_id IS NOT NULL;

-- Users already seeded get the new fixed Ahorro category too (the API also
-- ensures it on use).
INSERT INTO personal_categories (user_id, kind, name)
SELECT DISTINCT user_id, 'Ahorro', 'Gastos futuros' FROM personal_categories
ON CONFLICT DO NOTHING;
