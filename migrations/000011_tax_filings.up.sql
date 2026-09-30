-- Monthly RESICO filings. One row per period (the period is the key): the
-- amounts are the declaration computed when it was registered, rounded to
-- cents; isr_rate keeps the bracket rate as configured. iva_due is negative
-- when the balance is in the taxpayer's favor. The payment is recorded later
-- (or at registration): the three paid columns are all set or all NULL, and
-- a filing without them is "pendiente". The invoices a filing includes are the
-- ones whose declaration_period equals its period (no array column).
CREATE TABLE tax_filings (
    period              text          PRIMARY KEY CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    filing_date         date          NOT NULL,
    income_collected    numeric(14,2) NOT NULL CHECK (income_collected >= 0),
    isr_rate            numeric       NOT NULL CHECK (isr_rate >= 0 AND isr_rate < 1),
    isr_accrued         numeric(14,2) NOT NULL CHECK (isr_accrued >= 0),
    isr_withheld        numeric(14,2) NOT NULL CHECK (isr_withheld >= 0),
    isr_due             numeric(14,2) NOT NULL CHECK (isr_due >= 0),
    iva_transferred     numeric(14,2) NOT NULL CHECK (iva_transferred >= 0),
    iva_withheld        numeric(14,2) NOT NULL CHECK (iva_withheld >= 0),
    iva_creditable      numeric(14,2) NOT NULL CHECK (iva_creditable >= 0),
    iva_due             numeric(14,2) NOT NULL,
    folio               text          NOT NULL DEFAULT '' CHECK (char_length(folio) <= 64),
    payment_date        date,
    isr_paid            numeric(14,2) CHECK (isr_paid IS NULL OR isr_paid >= 0),
    iva_paid            numeric(14,2) CHECK (iva_paid IS NULL OR iva_paid >= 0),
    expense_movement_id bigint        REFERENCES movements (id) ON DELETE SET NULL,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    CHECK ((payment_date IS NULL) = (isr_paid IS NULL) AND (payment_date IS NULL) = (iva_paid IS NULL)),
    CHECK (expense_movement_id IS NULL OR payment_date IS NOT NULL)
);

CREATE INDEX invoices_declaration_period_idx ON invoices (declaration_period) WHERE declaration_period IS NOT NULL;
