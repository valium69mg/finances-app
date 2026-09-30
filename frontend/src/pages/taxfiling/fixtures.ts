import type { Filing, TaxPreview } from "../../api/taxFiling";

/** October filing with the payment pending: 591.25 of ISR and 1,600.00 of IVA. */
export const FILING: Filing = {
  period: "2026-10",
  filing_date: "2026-11-05",
  due_date: "2026-11-17",
  income_collected: "53750.00",
  isr_rate: "0.011",
  isr_accrued: "591.25",
  isr_withheld: "0.00",
  isr_due: "591.25",
  iva_transferred: "1600.00",
  iva_withheld: "0.00",
  iva_acreditable: "0.00",
  iva_due: "1600.00",
  total_to_pay: "2191.25",
  folio: "ACUSE-1",
  status: "pendiente",
  payment: null,
  expense_movement_id: null,
  invoice_ids: [1, 2],
  created_at: "2026-11-05T12:00:00Z",
};

export const PAID_FILING: Filing = {
  ...FILING,
  period: "2026-09",
  due_date: "2026-10-17",
  folio: "",
  status: "pagada",
  payment: { date: "2026-10-12", isr_paid: "500.00", iva_paid: "100.00", total_paid: "600.00" },
  expense_movement_id: 12,
};

export const PREVIEW: TaxPreview = {
  period: "2026-10",
  due_date: "2026-11-17",
  income_collected: "53750.00",
  isr_rate: "0.011",
  isr_accrued: "591.25",
  isr_withheld: "0.00",
  isr_due: "591.25",
  iva_transferred: "1600.00",
  iva_withheld: "0.00",
  iva_acreditable: "0.00",
  iva_due: "1600.00",
  total_to_pay: "2191.25",
  export_base: "43750.00",
  invoices: [
    { id: 1, client_id: "usa", collection_date: "2026-10-15", currency: "USD", subtotal_mxn: "43750.00", state: "emitida", uuid: "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B" },
    { id: 2, client_id: "b", collection_date: "2026-10-20", currency: "MXN", subtotal_mxn: "10000.00", state: "emitida", uuid: null },
  ],
  warnings: [],
  filing: null,
};
