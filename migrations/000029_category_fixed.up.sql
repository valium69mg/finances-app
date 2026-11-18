-- A fixed expense category (rent, services) is paid about once per cycle, so the
-- dashboard keeps it out of the spending pace. Only Gasto categories can be fixed.
ALTER TABLE categories
    ADD COLUMN fixed boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT categories_fixed_expense_only CHECK (NOT fixed OR kind = 'Gasto');
