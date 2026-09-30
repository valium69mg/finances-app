CREATE TABLE settings (
    id                           smallint PRIMARY KEY CHECK (id = 1),
    salary_usd                   numeric NOT NULL,
    fx_rate_applied              numeric NOT NULL,
    morse_fee_rate               numeric NOT NULL,
    emergency_months             numeric NOT NULL,
    extra_income_estimate_mxn    numeric NOT NULL DEFAULT 0,
    budget_includes_extra_income boolean NOT NULL DEFAULT false,
    extra_income_split           jsonb   NOT NULL DEFAULT '{}'::jsonb,
    investment_allocation        jsonb   NOT NULL DEFAULT '[]'::jsonb,
    updated_at                   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE issuer (
    id          smallint PRIMARY KEY CHECK (id = 1),
    rfc         text NOT NULL,
    name        text NOT NULL,
    regimen     text NOT NULL DEFAULT '',
    postal_code text NOT NULL DEFAULT '',
    note        text NOT NULL DEFAULT ''
);

CREATE TABLE categories (
    name     text PRIMARY KEY,
    position integer NOT NULL,
    kind     text    NOT NULL CHECK (kind IN ('Ingreso', 'Gasto', 'Ahorro')),
    budget   numeric CHECK (budget IS NULL OR budget >= 0),
    includes text    NOT NULL DEFAULT '',
    keywords text[]  NOT NULL DEFAULT '{}'
);

CREATE TABLE clients (
    id              text PRIMARY KEY,
    position        integer NOT NULL,
    name            text    NOT NULL,
    type            text    NOT NULL DEFAULT '',
    currency        text    NOT NULL CHECK (currency IN ('USD', 'MXN')),
    iva_rate        numeric NOT NULL DEFAULT 0 CHECK (iva_rate >= 0),
    rfc             text    NOT NULL DEFAULT '',
    regimen         text    NOT NULL DEFAULT '',
    uso_cfdi        text    NOT NULL DEFAULT '',
    ret_isr_rate    numeric NOT NULL DEFAULT 0 CHECK (ret_isr_rate >= 0),
    ret_iva_rate    numeric NOT NULL DEFAULT 0 CHECK (ret_iva_rate >= 0),
    concepto        text    NOT NULL DEFAULT '',
    clave_prod_serv text    NOT NULL DEFAULT '',
    clave_unidad    text    NOT NULL DEFAULT '',
    address         text    NOT NULL DEFAULT '',
    tax_residence   text    NOT NULL DEFAULT '',
    contract        text    NOT NULL DEFAULT '',
    real_payer      text    NOT NULL DEFAULT ''
);

CREATE TABLE instruments (
    id       text PRIMARY KEY,
    position integer NOT NULL,
    name     text NOT NULL,
    type     text NOT NULL DEFAULT '',
    platform text NOT NULL DEFAULT ''
);

CREATE TABLE instrument_by_category (
    category_name text PRIMARY KEY,
    instrument_id text NOT NULL REFERENCES instruments (id)
);

CREATE TABLE resico_brackets (
    position integer PRIMARY KEY,
    upper    numeric NOT NULL CHECK (upper > 0),
    rate     numeric NOT NULL CHECK (rate >= 0)
);

CREATE TABLE payment_methods (
    name     text PRIMARY KEY,
    position integer NOT NULL
);

CREATE TABLE investment_pause (
    id            smallint PRIMARY KEY CHECK (id = 1),
    normal_budget numeric NOT NULL CHECK (normal_budget >= 0),
    resume_month  text    NOT NULL,
    note          text    NOT NULL DEFAULT ''
);

CREATE TABLE investment_pause_plan (
    month       text PRIMARY KEY,
    position    integer NOT NULL,
    plan_amount numeric NOT NULL CHECK (plan_amount >= 0)
);
