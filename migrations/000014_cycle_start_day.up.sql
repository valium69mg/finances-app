-- Personal budget cycle: the day the cycle starts in the month before its
-- label (see ledger Cycle). 0 keeps the calendar month, 1..31 is a day clamped
-- to the month length (31 is the last day). The default leaves every existing
-- installation on calendar months until the owner changes it.
ALTER TABLE settings
    ADD COLUMN cycle_start_day smallint NOT NULL DEFAULT 0
        CHECK (cycle_start_day BETWEEN 0 AND 31);
