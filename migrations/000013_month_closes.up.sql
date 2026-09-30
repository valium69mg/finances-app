-- Month closes: one immutable snapshot per period (the period is the key). The
-- totals and the emergency fund are structured NUMERIC columns, exact as
-- computed when the close was generated (no rounding, no scale). The
-- per-category rows, the leftover suggestion and the budget-adjustment hints
-- are JSONB documents whose money values are decimal strings. Editing the
-- movements later never touches a row; a close is only removed by an explicit
-- delete so it can be generated again.
CREATE TABLE month_closes (
    period                text        PRIMARY KEY CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    closed_at             timestamptz NOT NULL DEFAULT now(),
    income                numeric     NOT NULL,
    expenses              numeric     NOT NULL,
    savings               numeric     NOT NULL,
    available             numeric     NOT NULL,
    emergency_accumulated numeric     NOT NULL,
    emergency_goal        numeric     NOT NULL,
    tax_filing_status     text        CHECK (tax_filing_status IS NULL OR tax_filing_status IN ('ninguna', 'pendiente', 'pagada')),
    categories            jsonb       NOT NULL CHECK (jsonb_typeof(categories) = 'array'),
    suggestion            jsonb       CHECK (suggestion IS NULL OR jsonb_typeof(suggestion) = 'object'),
    adjustments           jsonb       NOT NULL CHECK (jsonb_typeof(adjustments) = 'array')
);
