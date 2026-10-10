-- Phase 12, step 5: wishlist of a household user. Her rows live in their own
-- table. The status (activa / convertida) is NOT stored: it is derived at read
-- time from the three links below, so deleting the converted row reactivates
-- the item with no extra write.
CREATE TABLE personal_wishlist_items (
    id                  bigserial     PRIMARY KEY,
    user_id             uuid          NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name                text          NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 120),
    price               numeric(14,2) NOT NULL CHECK (price > 0),
    category_id         bigint        NOT NULL,
    -- Constant: together with the composite foreign key it makes the database
    -- reject a category of another user or of a kind other than Gasto.
    category_kind       text          NOT NULL DEFAULT 'Gasto' CHECK (category_kind = 'Gasto'),
    store_url           text          CHECK (store_url IS NULL OR (char_length(store_url) <= 500 AND store_url ~* '^https?://')),
    priority            text          NOT NULL DEFAULT 'media' CHECK (priority IN ('alta', 'media', 'baja')),
    note                text          CHECK (note IS NULL OR char_length(note) <= 500),
    future_expense_id   bigint,
    expense_movement_id bigint,
    expense_request_id  bigint        REFERENCES expense_requests (id) ON DELETE SET NULL,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    updated_at          timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT personal_wishlist_one_link CHECK (
        num_nonnulls(future_expense_id, expense_movement_id, expense_request_id) <= 1),
    CONSTRAINT personal_wishlist_category_fk FOREIGN KEY (category_id, user_id, category_kind)
        REFERENCES personal_categories (id, user_id, kind),
    -- Deleting the target only clears the link; user_id is never nulled.
    CONSTRAINT personal_wishlist_future_fk FOREIGN KEY (future_expense_id, user_id)
        REFERENCES personal_future_expenses (id, user_id) ON DELETE SET NULL (future_expense_id),
    CONSTRAINT personal_wishlist_expense_fk FOREIGN KEY (expense_movement_id, user_id)
        REFERENCES personal_movements (id, user_id) ON DELETE SET NULL (expense_movement_id)
);

CREATE INDEX personal_wishlist_user_idx ON personal_wishlist_items (user_id, created_at DESC);
CREATE INDEX personal_wishlist_request_idx ON personal_wishlist_items (expense_request_id) WHERE expense_request_id IS NOT NULL;
