DROP INDEX IF EXISTS movements_transfer_id_idx;
ALTER TABLE movements DROP COLUMN IF EXISTS transfer_id;
