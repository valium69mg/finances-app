-- Optional documents of a tax filing: the SAT acuse (PDF) and the payment proof
-- (PDF or image). The bytes live in the object store under key; only the
-- metadata is stored here. One current file per filing and kind: uploading
-- again replaces the row. A document is allowed on pending and paid filings
-- (it changes no figure) and goes away with its filing.
CREATE TABLE tax_filing_documents (
    period       text        NOT NULL REFERENCES tax_filings (period) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('acuse', 'comprobante')),
    key          text        NOT NULL UNIQUE,
    name         text        NOT NULL,
    content_type text        NOT NULL,
    size         bigint      NOT NULL CHECK (size > 0),
    uploaded_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (period, kind)
);
