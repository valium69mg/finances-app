-- Recurring bills and subscriptions. A bill has either a fixed amount or none
-- (variable). next_due_date is the due date of the bill's pending occurrence and
-- anchor_day the day of the month its schedule is based on (a bill due on the
-- 31st clamps to the last day of shorter months without drifting). Deactivating
-- a bill sets active = false; nothing is deleted.
CREATE TABLE bills (
    id                 bigserial     PRIMARY KEY,
    name               text          NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 120),
    category           text          NOT NULL CHECK (btrim(category) <> ''),
    amount             numeric(14,2) CHECK (amount IS NULL OR amount > 0),
    currency           text          NOT NULL DEFAULT 'MXN' CHECK (currency IN ('MXN', 'USD')),
    recurrence         text          NOT NULL CHECK (recurrence IN ('weekly', 'biweekly', 'monthly', 'bimonthly', 'yearly')),
    next_due_date      date          NOT NULL,
    anchor_day         smallint      NOT NULL CHECK (anchor_day BETWEEN 1 AND 31),
    reminder_lead_days integer       NOT NULL DEFAULT 3 CHECK (reminder_lead_days BETWEEN 0 AND 365),
    active             boolean       NOT NULL DEFAULT true,
    notes              text          NOT NULL DEFAULT '' CHECK (char_length(notes) <= 1000),
    created_at         timestamptz   NOT NULL DEFAULT now()
);

-- One row per due date. A bill has at most one pending occurrence (the next
-- one); paid and skipped occurrences are the history. A paid occurrence keeps
-- the amount paid and the expense it registered (NULL if that expense is
-- deleted later); a skipped one registers no expense.
CREATE TABLE bill_occurrences (
    id                  bigserial     PRIMARY KEY,
    bill_id             bigint        NOT NULL REFERENCES bills (id) ON DELETE CASCADE,
    due_date            date          NOT NULL,
    status              text          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'skipped')),
    paid_on             date,
    expense_movement_id bigint        REFERENCES movements (id) ON DELETE SET NULL,
    amount_paid         numeric(14,2) CHECK (amount_paid IS NULL OR amount_paid > 0),
    currency            text          CHECK (currency IS NULL OR currency IN ('MXN', 'USD')),
    resolved_at         timestamptz,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    CHECK (
        (status = 'pending' AND paid_on IS NULL AND amount_paid IS NULL AND currency IS NULL AND expense_movement_id IS NULL AND resolved_at IS NULL)
        OR (status = 'paid' AND paid_on IS NOT NULL AND amount_paid IS NOT NULL AND currency IS NOT NULL AND resolved_at IS NOT NULL)
        OR (status = 'skipped' AND paid_on IS NULL AND amount_paid IS NULL AND currency IS NULL AND expense_movement_id IS NULL AND resolved_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX bill_occurrences_one_pending_idx ON bill_occurrences (bill_id) WHERE status = 'pending';
CREATE INDEX bill_occurrences_history_idx ON bill_occurrences (bill_id, due_date DESC);
