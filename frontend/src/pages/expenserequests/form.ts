import type { ApproveInput, NewRequestInput, RequestDestination } from "../../api/expenseRequests";
import { isValuationAmount } from "../expenses/money";

export const MAX_DESCRIPTION_LENGTH = 120;
export const MAX_COMMENT_LENGTH = 500;
export const AMOUNT_MESSAGE = "Escribe un monto mayor a cero, con máximo 2 decimales, por ejemplo 250 o 250.50.";

const isDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v);

/** Editable state of the "Nueva petición" form. */
export interface RequestDraft {
  amount: string;
  description: string;
  /** "" means no suggestion. */
  category: string;
  /** YYYY-MM-DD */
  date: string;
}

export type RequestErrors = Partial<Record<"amount" | "description" | "date", string>>;

export const emptyDraft = (today: string): RequestDraft => ({ amount: "", description: "", category: "", date: today });

export function validateRequest(d: RequestDraft): RequestErrors {
  const errors: RequestErrors = {};
  if (!isValuationAmount(d.amount)) errors.amount = AMOUNT_MESSAGE;
  const description = d.description.trim();
  if (description === "") errors.description = "Describe en qué quieres gastar, por ejemplo Tacos o Gasolina.";
  else if ([...description].length > MAX_DESCRIPTION_LENGTH) errors.description = `La descripción puede tener hasta ${MAX_DESCRIPTION_LENGTH} caracteres.`;
  if (!isDate(d.date)) errors.date = "Elige la fecha del gasto.";
  return errors;
}

export const toNewRequestInput = (d: RequestDraft): NewRequestInput => ({
  amount: d.amount.trim(),
  description: d.description.trim(),
  date: d.date,
  ...(d.category !== "" ? { suggested_category: d.category } : {}),
});

/** Editable state of the approve dialog. */
export interface ApproveDraft {
  destination: RequestDestination;
  category: string;
  /** Date of the Gasto, YYYY-MM-DD. */
  date: string;
  paymentMethod: string;
  /** Due date of the future expense, YYYY-MM-DD. */
  dueDate: string;
}

export type ApproveErrors = Partial<Record<"category" | "date" | "dueDate", string>>;

export function validateApproval(d: ApproveDraft): ApproveErrors {
  const errors: ApproveErrors = {};
  if (d.destination === "gasto") {
    if (d.category === "") errors.category = "Elige la categoría del gasto.";
    if (!isDate(d.date)) errors.date = "Elige la fecha del gasto.";
  } else if (!isDate(d.dueDate)) {
    errors.dueDate = "Elige la fecha de vencimiento.";
  }
  return errors;
}

export function toApproveInput(d: ApproveDraft): ApproveInput {
  if (d.destination === "gasto_futuro") return { destination: "gasto_futuro", due_date: d.dueDate };
  return { destination: "gasto", category: d.category, date: d.date, ...(d.paymentMethod !== "" ? { payment_method: d.paymentMethod } : {}) };
}

/** The comment of a rejection: required and bounded. */
export function validateComment(comment: string): string | null {
  const c = comment.trim();
  if (c === "") return "Escribe el motivo del rechazo: se lo enviaremos a quien hizo la petición.";
  if ([...c].length > MAX_COMMENT_LENGTH) return `El comentario puede tener hasta ${MAX_COMMENT_LENGTH} caracteres.`;
  return null;
}
