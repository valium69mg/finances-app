CREATE TABLE movements (
    id             bigserial PRIMARY KEY,
    date           date        NOT NULL,
    description    text        NOT NULL DEFAULT '',
    category       text        NOT NULL,
    instrument     text,
    kind           text        NOT NULL CHECK (kind IN ('Ingreso', 'Gasto', 'Ahorro')),
    payment_method text        NOT NULL,
    currency       text        NOT NULL CHECK (currency IN ('MXN', 'USD')),
    amount         numeric     NOT NULL,
    exchange_rate  numeric     CHECK (exchange_rate IS NULL OR exchange_rate > 0),
    amount_mxn     numeric     NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CHECK ((currency = 'USD') = (exchange_rate IS NOT NULL)),
    CHECK (kind <> 'Gasto' OR amount > 0)
);

CREATE INDEX movements_date_idx ON movements (date);
CREATE INDEX movements_kind_date_idx ON movements (kind, date);
