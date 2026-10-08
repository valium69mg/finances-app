-- Postal code of the client (the receiver of the CFDI). Empty means not set:
-- public-in-general invoices use the issuer's, any other client is flagged as
-- missing configuration in the invoice checklist.
ALTER TABLE clients ADD COLUMN postal_code text NOT NULL DEFAULT '';
