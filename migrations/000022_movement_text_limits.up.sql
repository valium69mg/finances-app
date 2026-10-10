-- Bound the free text of a movement, like bills, future expenses and expense
-- requests. Added NOT VALID: rows that predate the limits are not checked (and
-- the migration cannot fail on them), while every new or updated row is.
ALTER TABLE movements
    ADD CONSTRAINT movements_description_length CHECK (char_length(description) <= 200) NOT VALID,
    ADD CONSTRAINT movements_instrument_length CHECK (instrument IS NULL OR char_length(instrument) <= 120) NOT VALID;
