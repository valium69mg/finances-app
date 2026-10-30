-- cycle_start_day 32 means "the last weekday (Mon-Fri) of the month" (see
-- ledger Cycle). Widen the CHECK from 0..31 to 0..32.
ALTER TABLE settings DROP CONSTRAINT settings_cycle_start_day_check;
ALTER TABLE settings
    ADD CONSTRAINT settings_cycle_start_day_check CHECK (cycle_start_day BETWEEN 0 AND 32);
