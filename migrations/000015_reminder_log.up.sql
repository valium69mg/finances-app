-- Dedupe log of the reminder emails already sent. The key identifies one email
-- (for example bill_due:<occurrence id> or weekly:<date>); the primary key makes
-- recording it race-safe, and sent_at lets the disk alert check when it last fired.
CREATE TABLE reminder_log (
    key     text        PRIMARY KEY,
    sent_at timestamptz NOT NULL DEFAULT now()
);
