-- Household step 2: roles, activation and movement authorship. Existing users
-- (the admin) stay owners and active; existing movements have no author.
ALTER TABLE users
    ADD COLUMN role   text    NOT NULL DEFAULT 'owner' CHECK (role IN ('owner', 'household')),
    ADD COLUMN active boolean NOT NULL DEFAULT true;

ALTER TABLE movements
    ADD COLUMN created_by uuid REFERENCES users (id) ON DELETE SET NULL;

CREATE INDEX movements_created_by_idx ON movements (created_by) WHERE created_by IS NOT NULL;
