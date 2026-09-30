import { api } from "./client";

/**
 * Expenses API. Amounts and rates travel as decimal strings and are never
 * converted to numbers, so no precision is lost.
 */

export interface Expense {
  id: number;
  /** YYYY-MM-DD */
  date: string;
  description: string;
  category: string;
  payment_method: string;
  currency: string;
  amount: string;
  /** Decimal string, or null for MXN expenses. */
  exchange_rate: string | null;
  amount_mxn: string;
}

/** Body for create and update. Only `amount` is required; the backend fills the rest. */
export interface ExpenseInput {
  date?: string;
  description?: string;
  category?: string;
  payment_method?: string;
  currency?: string;
  amount: string;
  exchange_rate?: string;
}

export interface BudgetFeedback {
  /** YYYY-MM */
  month: string;
  category: string;
  /** Decimal string, or null when the category has no budget. */
  budget: string | null;
  spent: string;
  remaining: string | null;
  over_budget: boolean;
}

export interface SaveExpenseResult {
  expense: Expense;
  budget: BudgetFeedback | null;
}

export const expensesKeys = {
  all: ["expenses"] as const,
  list: (month: string) => ["expenses", "list", month] as const,
  infer: (description: string) => ["expenses", "infer", description] as const,
};

type Client = Pick<typeof api, "request">;

export function createExpensesApi(client: Client = api) {
  return {
    list: (month: string, limit = 200) =>
      client.request<Expense[]>(`/expenses?month=${encodeURIComponent(month)}&limit=${limit}`),
    create: (input: ExpenseInput) => client.request<SaveExpenseResult>("/expenses", { method: "POST", body: input }),
    update: (id: number, input: ExpenseInput) =>
      client.request<SaveExpenseResult>(`/expenses/${id}`, { method: "PUT", body: input }),
    remove: (id: number) => client.request<void>(`/expenses/${id}`, { method: "DELETE" }),
    /** Suggested category for a description, or null when nothing matches. */
    inferCategory: async (description: string) => {
      const res = await client.request<{ category: string | null }>(
        `/expenses/infer-category?description=${encodeURIComponent(description)}`,
      );
      return res.category;
    },
  };
}

const defaultApi = createExpensesApi();

export const listExpenses = defaultApi.list;
export const createExpense = defaultApi.create;
export const updateExpense = defaultApi.update;
export const deleteExpense = defaultApi.remove;
export const inferCategory = defaultApi.inferCategory;
