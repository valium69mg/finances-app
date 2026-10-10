ALTER TABLE movements
    DROP CONSTRAINT IF EXISTS movements_description_length,
    DROP CONSTRAINT IF EXISTS movements_instrument_length;
