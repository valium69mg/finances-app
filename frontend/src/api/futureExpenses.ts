import { api } from "./client";

/**
 * Future expenses API ("Gastos futuros"): things the owner chooses to save for.
 * Amounts (MXN only) travel as decimal strings and are never converted to numbers.
 */

export type FutureExpenseStatus = "active" | "paid";

export interface FutureExpenseItem {
  id: number;
  name: string;
  target_amount: string;
  /** YYYY-MM-DD */
  due_date: string;
  status: FutureExpenseStatus;
  /** Net of the savings linked to the item (zero once paid). */
  saved: string;
  remaining: string;
  /** Amount to put aside each pay cycle to have it on time, rounded up to the cent. Zero when paid. */
  suggested_monthly: string;
  cycles_left: number;
  /** YYYY-MM-DD, only when paid. */
  paid_at: string | null;
  /** What was actually paid, only when paid. */
  amount_paid: string | null;
  /** The Gasto registered by the payment; null when unpaid or when that expense was deleted later. */
  expense_movement_id: number | null;
  created_at: string;
  updated_at: string;
}

export interface FutureExpenseTotals {
  target: string;
  saved: string;
  remaining: string;
  suggested_monthly: string;
}

export interface FutureExpensesList {
  /** Active items, earliest due date first. */
  active: FutureExpenseItem[];
  /** Paid items, most recently paid first. */
  paid: FutureExpenseItem[];
  /** Totals of the active items. */
  totals: FutureExpenseTotals;
  /** Gastos futuros savings linked to no item yet. */
  free_balance: string;
}

/** Body of create and update. */
export interface FutureExpenseInput {
  name: string;
  target_amount: string;
  due_date: string;
}

export interface SavingInput {
  amount: string;
  date?: string;
}

export interface PayFutureInput {
  /** What was actually paid; defaults to the target. */
  amount?: string;
  date?: string;
  /** Gasto category; empty lets the backend infer it from the name. */
  category?: string;
}

export interface RegisteredMovement {
  id: number;
  date: string;
  description: string;
  category: string;
  amount: string;
  amount_mxn: string;
}

export interface SavingResult {
  item: FutureExpenseItem;
  saving: RegisteredMovement;
}

export interface PayFutureResult {
  item: FutureExpenseItem;
  expense: RegisteredMovement;
}

export const futureExpensesKeys = {
  all: ["future-expenses"] as const,
  list: ["future-expenses", "list"] as const,
};

type Client = Pick<typeof api, "request">;

export function createFutureExpensesApi(client: Client = api) {
  return {
    list: () => client.request<FutureExpensesList>("/future-expenses"),
    create: (input: FutureExpenseInput) => client.request<FutureExpenseItem>("/future-expenses", { method: "POST", body: input }),
    update: (id: number, input: FutureExpenseInput) => client.request<FutureExpenseItem>(`/future-expenses/${id}`, { method: "PUT", body: input }),
    /** The savings linked to the item are kept: they return to the free balance. */
    remove: (id: number) => client.request<void>(`/future-expenses/${id}`, { method: "DELETE" }),
    addSaving: (id: number, input: SavingInput) => client.request<SavingResult>(`/future-expenses/${id}/savings`, { method: "POST", body: input }),
    /** Moves part of the free balance to the item (two Ahorro rows, atomically). */
    assign: (id: number, input: SavingInput) => client.request<FutureExpenseItem>(`/future-expenses/${id}/assign`, { method: "POST", body: input }),
    pay: (id: number, input: PayFutureInput) => client.request<PayFutureResult>(`/future-expenses/${id}/pay`, { method: "POST", body: input }),
  };
}

const defaultApi = createFutureExpensesApi();

export const listFutureExpenses = defaultApi.list;
export const createFutureExpense = defaultApi.create;
export const updateFutureExpense = defaultApi.update;
export const deleteFutureExpense = defaultApi.remove;
export const addFutureSaving = defaultApi.addSaving;
export const assignFutureSaving = defaultApi.assign;
export const payFutureExpense = defaultApi.pay;
