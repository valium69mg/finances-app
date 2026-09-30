CREATE TABLE valuations (
    id            bigserial PRIMARY KEY,
    date          date          NOT NULL,
    instrument_id text          NOT NULL,
    value_mxn     numeric(14,2) NOT NULL CHECK (value_mxn > 0),
    note          text          NOT NULL DEFAULT '',
    created_at    timestamptz   NOT NULL DEFAULT now()
);

CREATE INDEX valuations_instrument_date_idx ON valuations (instrument_id, date);
