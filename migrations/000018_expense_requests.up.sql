-- Household step 3: expense requests. A household user asks for an expense; the
-- owner approves it (as a Gasto or as a future expense), rejects it with a
-- comment, or the requester cancels it while it is still pending. Amounts are
-- MXN only in v1. A decided request is immutable: the CHECKs tie the decision
-- columns to the status, and the repository only transitions out of
-- 'solicitada'. The links to the created rows are SET NULL so deleting the
-- Gasto or the future expense later never blocks nor erases the request.
CREATE TABLE expense_requests (
    id                        bigserial     PRIMARY KEY,
    requester_id              uuid          NOT NULL REFERENCES users (id),
    amount                    numeric(14,2) NOT NULL CHECK (amount > 0),
    description               text          NOT NULL CHECK (btrim(description) <> '' AND char_length(description) <= 120),
    suggested_category        text          CHECK (suggested_category IS NULL OR (btrim(suggested_category) <> '' AND char_length(suggested_category) <= 80)),
    expense_date              date          NOT NULL DEFAULT CURRENT_DATE,
    status                    text          NOT NULL DEFAULT 'solicitada'
                                            CHECK (status IN ('solicitada', 'aprobada', 'rechazada', 'cancelada')),
    decision_comment          text          CHECK (decision_comment IS NULL OR (btrim(decision_comment) <> '' AND char_length(decision_comment) <= 500)),
    decided_by                uuid          REFERENCES users (id),
    decided_at                timestamptz,
    result_kind               text          CHECK (result_kind IS NULL OR result_kind IN ('gasto', 'gasto_futuro')),
    result_movement_id        bigint        REFERENCES movements (id) ON DELETE SET NULL,
    result_future_expense_id  bigint        REFERENCES future_expenses (id) ON DELETE SET NULL,
    created_at                timestamptz   NOT NULL DEFAULT now(),
    updated_at                timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT expense_requests_decision_matches_status CHECK (
        (status = 'solicitada'
            AND decided_by IS NULL AND decided_at IS NULL AND decision_comment IS NULL
            AND result_kind IS NULL AND result_movement_id IS NULL AND result_future_expense_id IS NULL)
        OR (status = 'cancelada'
            AND decided_by IS NULL AND decided_at IS NOT NULL AND decision_comment IS NULL
            AND result_kind IS NULL AND result_movement_id IS NULL AND result_future_expense_id IS NULL)
        OR (status = 'rechazada'
            AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND decision_comment IS NOT NULL
            AND result_kind IS NULL AND result_movement_id IS NULL AND result_future_expense_id IS NULL)
        OR (status = 'aprobada'
            AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND result_kind IS NOT NULL
            AND (result_kind = 'gasto' OR result_movement_id IS NULL)
            AND (result_kind = 'gasto_futuro' OR result_future_expense_id IS NULL))
    )
);

CREATE INDEX expense_requests_requester_created_idx ON expense_requests (requester_id, created_at);
CREATE INDEX expense_requests_status_idx ON expense_requests (status);
