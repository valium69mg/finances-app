-- Phase 12, steps 1-3: personal finances of a household user. Her rows live in
-- their own tables, never in movements or future_expenses, so no owner query
-- can mix them into the family totals. Every row carries user_id.
CREATE TABLE personal_categories (
    id         bigserial   PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       text        NOT NULL CHECK (kind IN ('Ingreso', 'Gasto', 'Ahorro')),
    name       text        NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 80),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT personal_categories_unique_name UNIQUE (user_id, kind, name),
    -- Target of the composite foreign key of personal_movements, so a movement
    -- can only point at a category of the same user and the same kind.
    CONSTRAINT personal_categories_owner_kind UNIQUE (id, user_id, kind)
);

CREATE TABLE personal_movements (
    id          bigserial     PRIMARY KEY,
    user_id     uuid          NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind        text          NOT NULL CHECK (kind IN ('Ingreso', 'Gasto', 'Ahorro')),
    category_id bigint        NOT NULL,
    amount      numeric(14,2) NOT NULL,
    description text          NOT NULL CHECK (btrim(description) <> '' AND char_length(description) <= 120),
    date        date          NOT NULL,
    created_at  timestamptz   NOT NULL DEFAULT now(),
    updated_at  timestamptz   NOT NULL DEFAULT now(),
    -- Ingreso and Gasto are positive; an Ahorro may be negative (a withdrawal).
    CONSTRAINT personal_movements_amount_sign CHECK (
        (kind = 'Ahorro' AND amount <> 0) OR (kind <> 'Ahorro' AND amount > 0)),
    CONSTRAINT personal_movements_category_fk FOREIGN KEY (category_id, user_id, kind)
        REFERENCES personal_categories (id, user_id, kind)
);

CREATE INDEX personal_movements_user_date ON personal_movements (user_id, date);
