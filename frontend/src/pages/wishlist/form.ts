import type { ConvertExpenseInput, ConvertFutureInput, ConvertRequestInput, WishInput, WishItem, WishPriority } from "../../api/wishlist";
import { isValuationAmount } from "../expenses/money";

export const MAX_NAME_LENGTH = 120;
export const MAX_URL_LENGTH = 500;
export const MAX_NOTE_LENGTH = 500;
export const PRICE_MESSAGE = "Escribe un precio mayor a cero, con máximo 2 decimales, por ejemplo 499 o 499.90.";
export const AMOUNT_MESSAGE = "Escribe un monto mayor a cero, con máximo 2 decimales, por ejemplo 499 o 499.90.";

const isDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v);

/** True for an http or https link with a host, the only kind the backend accepts. */
export function isStoreUrl(raw: string): boolean {
  const v = raw.trim();
  if (/\s/.test(v) || !/^https?:\/\//i.test(v)) return false;
  try {
    return new URL(v).hostname !== "";
  } catch {
    return false;
  }
}

/** Editable state of the create / edit form of an item. */
export interface WishDraft {
  name: string;
  price: string;
  /** Category id as the select holds it. */
  categoryId: string;
  storeUrl: string;
  priority: WishPriority;
  note: string;
}

export type WishErrors = Partial<Record<"name" | "price" | "category" | "storeUrl" | "note", string>>;

export const emptyDraft = (categoryId = ""): WishDraft => ({ name: "", price: "", categoryId, storeUrl: "", priority: "media", note: "" });

export const draftOf = (item: WishItem): WishDraft => ({
  name: item.name,
  price: item.price,
  categoryId: String(item.category_id),
  storeUrl: item.store_url ?? "",
  priority: item.priority,
  note: item.note ?? "",
});

export function validateWish(d: WishDraft): WishErrors {
  const errors: WishErrors = {};
  const name = d.name.trim();
  if (name === "") errors.name = "Escribe qué quieres, por ejemplo Bolsa o Zapatos.";
  else if ([...name].length > MAX_NAME_LENGTH) errors.name = `El nombre puede tener hasta ${MAX_NAME_LENGTH} caracteres.`;
  if (!isValuationAmount(d.price)) errors.price = PRICE_MESSAGE;
  if (d.categoryId === "") errors.category = "Elige una categoría.";
  const url = d.storeUrl.trim();
  if (url !== "") {
    if ([...url].length > MAX_URL_LENGTH) errors.storeUrl = `El enlace puede tener hasta ${MAX_URL_LENGTH} caracteres.`;
    else if (!isStoreUrl(url)) errors.storeUrl = "Escribe un enlace que empiece con http:// o https://.";
  }
  if ([...d.note.trim()].length > MAX_NOTE_LENGTH) errors.note = `La nota puede tener hasta ${MAX_NOTE_LENGTH} caracteres.`;
  return errors;
}

export const toWishInput = (d: WishDraft): WishInput => ({
  name: d.name.trim(),
  price: d.price.trim(),
  category_id: Number(d.categoryId),
  priority: d.priority,
  ...(d.storeUrl.trim() !== "" ? { store_url: d.storeUrl.trim() } : {}),
  ...(d.note.trim() !== "" ? { note: d.note.trim() } : {}),
});

/** The three ways an active item converts. */
export type ConvertChoice = "gasto_futuro" | "gasto" | "peticion";

/** Editable state of the "Convertir" dialog; only the fields of the chosen way are used. */
export interface ConvertDraft {
  choice: ConvertChoice;
  /** Future expense: amount to save (defaults to the price) and due date. */
  target: string;
  dueDate: string;
  /** Gasto and request: amount (defaults to the price) and date. */
  amount: string;
  date: string;
  /** Gasto: id of one of her categories (defaults to the item's). */
  categoryId: string;
  /** Request: name of an owner Gasto category. */
  ownerCategory: string;
}

export type ConvertErrors = Partial<Record<"target" | "dueDate" | "amount" | "date" | "category" | "ownerCategory", string>>;

export const defaultConvertDraft = (item: WishItem, today: string): ConvertDraft => ({
  choice: "gasto_futuro",
  target: item.price,
  dueDate: "",
  amount: item.price,
  date: today,
  categoryId: String(item.category_id),
  ownerCategory: "",
});

export function validateConversion(d: ConvertDraft): ConvertErrors {
  const errors: ConvertErrors = {};
  if (d.choice === "gasto_futuro") {
    if (!isValuationAmount(d.target)) errors.target = AMOUNT_MESSAGE;
    if (!isDate(d.dueDate)) errors.dueDate = "Elige la fecha de vencimiento.";
    return errors;
  }
  if (!isValuationAmount(d.amount)) errors.amount = AMOUNT_MESSAGE;
  if (!isDate(d.date)) errors.date = "Elige la fecha.";
  if (d.choice === "gasto" && d.categoryId === "") errors.category = "Elige la categoría del gasto.";
  if (d.choice === "peticion" && d.ownerCategory === "") errors.ownerCategory = "Elige la categoría con la que se pedirá.";
  return errors;
}

export const toFutureInput = (d: ConvertDraft): ConvertFutureInput => ({ target_amount: d.target.trim(), due_date: d.dueDate });

export const toExpenseInput = (d: ConvertDraft): ConvertExpenseInput => ({ amount: d.amount.trim(), date: d.date, category_id: Number(d.categoryId) });

export const toRequestInput = (d: ConvertDraft): ConvertRequestInput => ({ amount: d.amount.trim(), date: d.date, category: d.ownerCategory });
