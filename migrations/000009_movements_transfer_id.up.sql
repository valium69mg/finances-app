ALTER TABLE movements ADD COLUMN transfer_id uuid;

-- Both legs of a savings transfer share the id; ordinary movements keep NULL.
CREATE INDEX movements_transfer_id_idx ON movements (transfer_id) WHERE transfer_id IS NOT NULL;
