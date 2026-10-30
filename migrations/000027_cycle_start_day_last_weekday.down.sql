-- Rows holding 32 (last weekday) have no equivalent below 32: they fall back
-- to 31 (last day of the month) so the narrower CHECK can be restored.
UPDATE settings SET cycle_start_day = 31 WHERE cycle_start_day = 32;
ALTER TABLE settings DROP CONSTRAINT settings_cycle_start_day_check;
ALTER TABLE settings
    ADD CONSTRAINT settings_cycle_start_day_check CHECK (cycle_start_day BETWEEN 0 AND 31);
