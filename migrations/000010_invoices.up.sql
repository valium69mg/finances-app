-- Invoices prepared for the SAT portal and their issued CFDI files. Only the
-- metadata of a file lives here; the bytes are in S3-compatible storage under
-- invoice_documents.key. Money is rounded to cents (NUMERIC(14,2)); exchange_rate
-- keeps its full precision, as in movements.
CREATE TABLE invoices (
    id                    bigserial     PRIMARY KEY,
    client_id             text          NOT NULL,
    collection_date       date          NOT NULL,
    period                text          NOT NULL CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    currency              text          NOT NULL CHECK (currency IN ('MXN', 'USD')),
    exchange_rate         numeric       CHECK (exchange_rate IS NULL OR exchange_rate > 0),
    subtotal              numeric(14,2) NOT NULL CHECK (subtotal > 0),
    subtotal_mxn          numeric(14,2) NOT NULL CHECK (subtotal_mxn > 0),
    iva                   numeric(14,2) NOT NULL DEFAULT 0,
    isr_withheld          numeric(14,2) NOT NULL DEFAULT 0,
    iva_withheld          numeric(14,2) NOT NULL DEFAULT 0,
    total                 numeric(14,2) NOT NULL CHECK (total > 0),
    expected_deposit_mxn  numeric(14,2) NOT NULL,
    status                text          NOT NULL DEFAULT 'preparada'
                                        CHECK (status IN ('preparada', 'emitida', 'cancelada')),
    uuid                  text          UNIQUE CHECK (uuid IS NULL OR uuid ~ '^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$'),
    movement_id           bigint        REFERENCES movements (id) ON DELETE SET NULL,
    -- Set by the future Tax Filing module when a declaration includes the invoice.
    declaration_period    text          CHECK (declaration_period IS NULL OR declaration_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    created_at            timestamptz   NOT NULL DEFAULT now(),
    CHECK ((currency = 'USD') = (exchange_rate IS NOT NULL)),
    CHECK (status <> 'emitida' OR uuid IS NOT NULL)
);

CREATE INDEX invoices_period_idx ON invoices (period);
CREATE INDEX invoices_client_date_idx ON invoices (client_id, collection_date);

CREATE TABLE invoice_documents (
    id           bigserial   PRIMARY KEY,
    invoice_id   bigint      NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('xml', 'pdf')),
    key          text        NOT NULL UNIQUE,
    name         text        NOT NULL,
    content_type text        NOT NULL,
    size         bigint      NOT NULL CHECK (size > 0),
    sha256       text        NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    uploaded_at  timestamptz NOT NULL DEFAULT now(),
    -- One current file per kind: replacing a document swaps the row.
    UNIQUE (invoice_id, kind)
);
