import { api } from "./client";
import type { MovementKind } from "./dashboard";
import type { FutureExpenseInput, FutureExpenseItem, FutureExpensesList, SavingInput } from "./futureExpenses";

/**
 * Personal finances API (household role): her own categories and movements, fully separate from the family
 * ledger. Amounts travel as decimal strings. An Ahorro amount is negative for a withdrawal.
 */

export interface PersonalCategory {
  id: number;
  kind: MovementKind;
  name: string;
  /** Fixed categories (Ingreso and Ahorro) cannot be renamed. */
  fixed: boolean;
}

export interface PersonalMovement {
  id: number;
  kind: MovementKind;
  category_id: number;
  category: string;
  /** Decimal string; negative only for an Ahorro withdrawal. */
  amount: string;
  description: string;
  /** YYYY-MM-DD */
  date: string;
  /** The future expense an Ahorro feeds, null otherwise. A manual update never changes it. */
  future_expense_id: number | null;
  created_at: string;
  updated_at: string;
}

/** Body of the payment of one of her future expenses: the Gasto category is required and is one of hers. */
export interface PayPersonalFutureInput {
  /** What was actually paid; defaults to the target. */
  amount?: string;
  date?: string;
  category_id: number;
}

export interface PersonalSavingResult {
  item: FutureExpenseItem;
  saving: PersonalMovement;
}

export interface PersonalPayResult {
  item: FutureExpenseItem;
  expense: PersonalMovement;
}

/** Body for create and update. An empty or missing date means today on the server. */
export interface PersonalMovementInput {
  kind: MovementKind;
  category_id: number;
  amount: string;
  description?: string;
  date?: string;
}

export interface PersonalListFilter {
  /** The owner's read-only view: the household user whose rows are listed. Her own session leaves it out. */
  userId?: string;
  /** YYYY-MM cycle label; empty or missing lets the server pick the current cycle. */
  month?: string;
  kind?: MovementKind;
  categoryId?: number;
}

/** A pay cycle: the YYYY-MM label and its inclusive YYYY-MM-DD bounds (the owner's cut day, which she cannot read). */
export interface PersonalPeriod {
  month: string;
  period_start: string;
  period_end: string;
}

/** One Gasto category of hers with what it spent in the cycle. */
export interface PersonalCategorySpend {
  category_id: number;
  category: string;
  /** Decimal string, always positive. */
  spent: string;
}

/** Her summary of one cycle. `savings` is net of withdrawals and `available` = income - expenses - savings. */
export interface PersonalDashboard extends PersonalPeriod {
  income: string;
  expenses: string;
  savings: string;
  available: string;
  /** Only categories with spending in the cycle, biggest first. */
  categories: PersonalCategorySpend[];
}

/**
 * Query keys. The read ones take the optional `userId` of the owner's read-only view and append it, so the cache of
 * one household user is never served for another. Her own keys (no userId) keep their shape.
 */
const scoped = <T extends readonly unknown[]>(key: T, userId?: string) => (userId ? ([...key, userId] as const) : key);

export const personalKeys = {
  all: ["personal"] as const,
  categories: (kind: MovementKind, userId?: string) => scoped(["personal", "categories", kind] as const, userId),
  movements: (kind: MovementKind, month: string, categoryId = 0, userId?: string) =>
    scoped(["personal", "movements", kind, month, categoryId] as const, userId),
  /** An empty month lets the server pick the current cycle. */
  dashboard: (month: string, userId?: string) => scoped(["personal", "dashboard", month] as const, userId),
  period: (month: string, userId?: string) => scoped(["personal", "period", month] as const, userId),
  futureExpenses: ["personal", "future-expenses"] as const,
  futureExpensesOf: (userId: string) => ["personal", "future-expenses", userId] as const,
};

/** `path?a=b&user_id=<id>`: the query of a read, with the target user when the owner is looking. */
function readPath(path: string, params: Record<string, string | undefined>, userId?: string): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
  if (userId) q.set("user_id", userId);
  const query = q.toString();
  return query ? `${path}?${query}` : path;
}

type Client = Pick<typeof api, "request">;

export function createPersonalApi(client: Client = api) {
  return {
    /** Her summary of a cycle; an empty month asks for the current one. */
    dashboard: (month: string, userId?: string) => client.request<PersonalDashboard>(readPath("/personal/dashboard", { month }, userId)),
    /** The range of a cycle, for the hint of her month pickers; an empty month asks for the current one. */
    period: (month: string, userId?: string) => client.request<PersonalPeriod>(readPath("/personal/period", { month }, userId)),
    categories: (kind: MovementKind, userId?: string) => client.request<PersonalCategory[]>(readPath("/personal/categories", { kind }, userId)),
    createCategory: (name: string) => client.request<PersonalCategory>("/personal/categories", { method: "POST", body: { name } }),
    renameCategory: (id: number, name: string) => client.request<PersonalCategory>(`/personal/categories/${id}`, { method: "PUT", body: { name } }),
    movements: (filter: PersonalListFilter = {}) => {
      const params = { month: filter.month, kind: filter.kind, category_id: filter.categoryId ? String(filter.categoryId) : undefined };
      return client.request<PersonalMovement[]>(readPath("/personal/movements", params, filter.userId));
    },
    createMovement: (input: PersonalMovementInput) => client.request<PersonalMovement>("/personal/movements", { method: "POST", body: input }),
    updateMovement: (id: number, input: PersonalMovementInput) =>
      client.request<PersonalMovement>(`/personal/movements/${id}`, { method: "PUT", body: input }),
    removeMovement: (id: number) => client.request<void>(`/personal/movements/${id}`, { method: "DELETE" }),

    // Her future expenses: same model and payloads as the owner's, under /personal/future-expenses.
    futureList: (userId?: string) => client.request<FutureExpensesList>(readPath("/personal/future-expenses", {}, userId)),
    futureCreate: (input: FutureExpenseInput) => client.request<FutureExpenseItem>("/personal/future-expenses", { method: "POST", body: input }),
    futureUpdate: (id: number, input: FutureExpenseInput) =>
      client.request<FutureExpenseItem>(`/personal/future-expenses/${id}`, { method: "PUT", body: input }),
    /** The savings linked to the item are kept: they return to the free balance. */
    futureRemove: (id: number) => client.request<void>(`/personal/future-expenses/${id}`, { method: "DELETE" }),
    futureAddSaving: (id: number, input: SavingInput) =>
      client.request<PersonalSavingResult>(`/personal/future-expenses/${id}/savings`, { method: "POST", body: input }),
    /** Moves part of the free balance to the item (two Ahorro rows, atomically). */
    futureAssign: (id: number, input: SavingInput) =>
      client.request<FutureExpenseItem>(`/personal/future-expenses/${id}/assign`, { method: "POST", body: input }),
    futurePay: (id: number, input: PayPersonalFutureInput) =>
      client.request<PersonalPayResult>(`/personal/future-expenses/${id}/pay`, { method: "POST", body: input }),
  };
}

const defaultApi = createPersonalApi();

export const getPersonalDashboard = defaultApi.dashboard;
export const getPersonalPeriod = defaultApi.period;
export const listPersonalCategories = defaultApi.categories;
export const createPersonalCategory = defaultApi.createCategory;
export const renamePersonalCategory = defaultApi.renameCategory;
export const listPersonalMovements = defaultApi.movements;
export const createPersonalMovement = defaultApi.createMovement;
export const updatePersonalMovement = defaultApi.updateMovement;
export const deletePersonalMovement = defaultApi.removeMovement;
export const listPersonalFutureExpenses = defaultApi.futureList;
export const createPersonalFutureExpense = defaultApi.futureCreate;
export const updatePersonalFutureExpense = defaultApi.futureUpdate;
export const deletePersonalFutureExpense = defaultApi.futureRemove;
export const addPersonalFutureSaving = defaultApi.futureAddSaving;
export const assignPersonalFutureSaving = defaultApi.futureAssign;
export const payPersonalFutureExpense = defaultApi.futurePay;
