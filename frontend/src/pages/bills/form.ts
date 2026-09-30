import type { Bill, BillInput, PayInput, Recurrence } from "../../api/bills";
import type { AllSettings } from "../../api/settings";
import { isValuationAmount } from "../expenses/money";

/** Names of the expense (Gasto) categories, the ones a bill can be filed under. */
export const expenseCategories = (settings: AllSettings | undefined): string[] =>
  (settings?.categories ?? []).filter((c) => c.kind === "Gasto").map((c) => c.name);

/** Editable state of the create / edit form. */
export interface BillDraft {
  name: string;
  category: string;
  /** A variable bill has no amount: it is typed every time it is paid. */
  variable: boolean;
  amount: string;
  currency: string;
  recurrence: Recurrence;
  /** YYYY-MM-DD */
  nextDueDate: string;
  leadDays: string;
  notes: string;
}

export type BillErrors = Partial<Record<"name" | "category" | "amount" | "date" | "lead", string>>;

export const DEFAULT_LEAD_DAYS = "3";
export const MAX_LEAD_DAYS = 365;

/** Empty form for a new bill; the due date starts today. */
export function emptyDraft(today: string, category = ""): BillDraft {
  return { name: "", category, variable: false, amount: "", currency: "MXN", recurrence: "monthly", nextDueDate: today, leadDays: DEFAULT_LEAD_DAYS, notes: "" };
}

/** Form state of an existing bill. */
export function draftOf(bill: Bill): BillDraft {
  return {
    name: bill.name,
    category: bill.category,
    variable: bill.amount === null,
    amount: bill.amount ?? "",
    currency: bill.currency,
    recurrence: bill.recurrence,
    nextDueDate: bill.next_due_date,
    leadDays: String(bill.reminder_lead_days),
    notes: bill.notes,
  };
}

const isDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v);
const isLead = (v: string) => /^\d{1,3}$/.test(v.trim()) && Number(v) <= MAX_LEAD_DAYS;

export const AMOUNT_MESSAGE = "Escribe un monto mayor a cero, con máximo 2 decimales, por ejemplo 550 o 550.50.";

/** Validates the amount of a payment or of a fixed-amount bill: at least 0.01, at most 2 decimals. */
export const isBillAmount = (v: string) => isValuationAmount(v);

export function validateBill(d: BillDraft): BillErrors {
  const errors: BillErrors = {};
  if (d.name.trim() === "") errors.name = "Escribe el nombre del pago, por ejemplo Megacable.";
  else if ([...d.name.trim()].length > 120) errors.name = "El nombre puede tener hasta 120 caracteres.";
  if (d.category === "") errors.category = "Elige la categoría del gasto.";
  if (!d.variable && !isBillAmount(d.amount)) errors.amount = AMOUNT_MESSAGE;
  if (!isDate(d.nextDueDate)) errors.date = "Elige la fecha del próximo vencimiento.";
  if (!isLead(d.leadDays)) errors.lead = `Escribe un número de días entre 0 y ${MAX_LEAD_DAYS}.`;
  return errors;
}

/** Request body of a valid draft. A variable bill sends a null amount. */
export function toBillInput(d: BillDraft, active: boolean): BillInput {
  return {
    name: d.name.trim(),
    category: d.category,
    amount: d.variable ? null : d.amount.trim(),
    currency: d.currency,
    recurrence: d.recurrence,
    next_due_date: d.nextDueDate,
    reminder_lead_days: Number(d.leadDays),
    active,
    notes: d.notes.trim(),
  };
}

/** Editable state of the payment dialog. */
export interface PayDraft {
  amount: string;
  /** YYYY-MM-DD */
  date: string;
  category: string;
  description: string;
}

export type PayErrors = Partial<Record<"amount" | "date" | "category", string>>;

/** The payment as the bill defines it: its fixed amount (empty for a variable bill), today and its category. */
export function defaultPayDraft(bill: Pick<Bill, "amount" | "category">, today: string): PayDraft {
  return { amount: bill.amount ?? "", date: today, category: bill.category, description: "" };
}

export function validatePay(d: PayDraft): PayErrors {
  const errors: PayErrors = {};
  if (!isBillAmount(d.amount)) errors.amount = AMOUNT_MESSAGE;
  if (!isDate(d.date)) errors.date = "Elige la fecha del pago.";
  if (d.category === "") errors.category = "Elige la categoría del gasto.";
  return errors;
}

/** Request body of a valid payment draft; the description is only sent when written. */
export function toPayInput(d: PayDraft): PayInput {
  const input: PayInput = { date: d.date, amount: d.amount.trim(), category: d.category };
  if (d.description.trim()) input.description = d.description.trim();
  return input;
}
