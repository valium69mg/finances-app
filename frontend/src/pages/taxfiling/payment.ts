import type { PaymentInput } from "../../api/taxFiling";

/** Editable state of the payment of a filing. */
export interface PaymentDraft {
  /** YYYY-MM-DD */
  date: string;
  isr: string;
  iva: string;
  recordExpense: boolean;
}

export type PaymentErrors = Partial<Record<"date" | "isr" | "iva" | "expense", string>>;

/** Amount the API accepts as paid: zero or more, at most 2 decimals and below 1,000,000,000,000. */
export const isPaidAmount = (v: string) => /^\d{1,12}(\.\d{1,2})?$/.test(v.trim());

/** True for a plain amount with a non-zero digit. */
const isPositiveAmount = (v: string) => isPaidAmount(v) && /[1-9]/.test(v);

/** Amount owed as an input default: a negative IVA balance (in favor) is 0 to pay. */
export const amountToPay = (due: string) => (due.trim().startsWith("-") ? "0.00" : due);

/** The payment as the SAT asked for it: the ISR and the IVA due, on the given date, without an expense. */
export function defaultPaymentDraft(isrDue: string, ivaDue: string, date: string): PaymentDraft {
  return { date, isr: amountToPay(isrDue), iva: amountToPay(ivaDue), recordExpense: false };
}

export function validatePayment(d: PaymentDraft): PaymentErrors {
  const errors: PaymentErrors = {};
  if (!/^\d{4}-\d{2}-\d{2}$/.test(d.date)) errors.date = "Elige la fecha de pago.";
  const amountMessage = "Escribe un monto de cero o más, con máximo 2 decimales, por ejemplo 1845.25.";
  if (!isPaidAmount(d.isr)) errors.isr = amountMessage;
  if (!isPaidAmount(d.iva)) errors.iva = amountMessage;
  if (d.recordExpense && !errors.isr && !errors.iva && !isPositiveAmount(d.isr) && !isPositiveAmount(d.iva)) {
    errors.expense = "Para registrar el gasto, el ISR o el IVA pagado debe ser mayor a cero.";
  }
  return errors;
}

/** Request body of a valid draft; record_expense is only sent when it is on. */
export function toPaymentInput(d: PaymentDraft): PaymentInput {
  const input: PaymentInput = { date: d.date, isr_paid: d.isr.trim(), iva_paid: d.iva.trim() };
  if (d.recordExpense) input.record_expense = true;
  return input;
}
