import type { FutureExpenseInput, FutureExpenseItem, PayFutureInput } from "../../api/futureExpenses";
import { isValuationAmount } from "../expenses/money";

export const MAX_NAME_LENGTH = 120;
export const AMOUNT_MESSAGE = "Escribe un monto mayor a cero, con máximo 2 decimales, por ejemplo 8000 u 8000.50.";

/** Editable state of the create / edit form of an item. */
export interface ItemDraft {
  name: string;
  target: string;
  /** YYYY-MM-DD */
  dueDate: string;
}

export type ItemErrors = Partial<Record<"name" | "target" | "date", string>>;

export const emptyDraft = (): ItemDraft => ({ name: "", target: "", dueDate: "" });

export const draftOf = (item: FutureExpenseItem): ItemDraft => ({ name: item.name, target: item.target_amount, dueDate: item.due_date });

const isDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v);

export function validateItem(d: ItemDraft): ItemErrors {
  const errors: ItemErrors = {};
  if (d.name.trim() === "") errors.name = "Escribe el nombre del gasto, por ejemplo Laptop o Vacaciones.";
  else if ([...d.name.trim()].length > MAX_NAME_LENGTH) errors.name = `El nombre puede tener hasta ${MAX_NAME_LENGTH} caracteres.`;
  if (!isValuationAmount(d.target)) errors.target = AMOUNT_MESSAGE;
  if (!isDate(d.dueDate)) errors.date = "Elige la fecha de vencimiento.";
  return errors;
}

export const toItemInput = (d: ItemDraft): FutureExpenseInput => ({ name: d.name.trim(), target_amount: d.target.trim(), due_date: d.dueDate });

/** Amount + date of a saving or an assignment. */
export interface AmountDraft {
  amount: string;
  date: string;
}

export type AmountErrors = Partial<Record<"amount" | "date", string>>;

export function validateAmount(d: AmountDraft): AmountErrors {
  const errors: AmountErrors = {};
  if (!isValuationAmount(d.amount)) errors.amount = AMOUNT_MESSAGE;
  if (!isDate(d.date)) errors.date = "Elige la fecha.";
  return errors;
}

/** Payment of an item: what was actually paid, when and the Gasto category ("" lets the backend infer it). */
export interface PayDraft extends AmountDraft {
  category: string;
}

export const defaultPayDraft = (item: FutureExpenseItem, today: string): PayDraft => ({ amount: item.target_amount, date: today, category: "" });

export const toPayInput = (d: PayDraft): PayFutureInput => ({
  amount: d.amount.trim(),
  date: d.date,
  ...(d.category !== "" ? { category: d.category } : {}),
});

/** Plain decimal strings compared without floats: is a greater than b? (both non-negative, at most 2 decimals) */
export function isGreater(a: string, b: string): boolean {
  const cents = (v: string) => {
    const m = /^(\d+)(?:\.(\d+))?$/.exec(v.trim());
    return m ? BigInt(m[1] + (m[2] ?? "").padEnd(2, "0").slice(0, 2)) : 0n;
  };
  return cents(a) > cents(b);
}

/** True for "0", "0.00", "" and any negative amount: there is nothing to assign. */
export const isEmptyBalance = (v: string) => !/^\d+(\.\d+)?$/.test(v.trim()) || !/[1-9]/.test(v);
