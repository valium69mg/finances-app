import { api } from "./client";

/**
 * Expense requests API ("Peticiones de gasto"). A household user asks for an expense; the owner approves it
 * (as a Gasto or as a future expense), rejects it with a comment, or the requester cancels it while it is pending.
 * Amounts (MXN only) travel as decimal strings and are never converted to numbers.
 */

export type RequestStatus = "solicitada" | "aprobada" | "rechazada" | "cancelada";

export const REQUEST_STATUSES: readonly RequestStatus[] = ["solicitada", "aprobada", "rechazada", "cancelada"];

export type RequestDestination = "gasto" | "gasto_futuro";

export interface ExpenseRequest {
  id: number;
  requester_email: string;
  amount: string;
  description: string;
  /** Only a suggestion: the owner picks the real category. */
  suggested_category: string | null;
  /** YYYY-MM-DD */
  expense_date: string;
  status: RequestStatus;
  /** Set when rejected. */
  decision_comment: string | null;
  decided_at: string | null;
  /** What approving created; null unless approved. */
  result_kind: RequestDestination | null;
  /** The Gasto, null when not registered as a Gasto or deleted later. */
  result_movement_id: number | null;
  /** The future expense, null when not moved there or deleted later. */
  result_future_expense_id: number | null;
  created_at: string;
  /** Audit trail: how many times the owner reverted an approval back to solicitada (0 when never). */
  revert_count: number;
  /** When it was last reverted, null when never. */
  reverted_at: string | null;
}

/** Body of POST /expense-requests. */
export interface NewRequestInput {
  amount: string;
  description: string;
  /** Name of a Gasto category; omitted when there is no suggestion. */
  suggested_category?: string;
  date?: string;
}

/** Owner only: what approving the request as a Gasto does to the budget of its category in that pay cycle. */
export interface BudgetCheck {
  category: string;
  /** Null when the category has no budget. */
  budget: string | null;
  spent: string;
  remaining: string | null;
  /** The amount of the request. */
  amount: string;
  projected_spent: string;
  projected_remaining: string | null;
  /** Null when there is no budget ("Sin presupuesto"). */
  fits: boolean | null;
  /** Positive excess over the budget, "0.00" when it fits. */
  over_by: string;
}

export type ApproveInput =
  | { destination: "gasto"; category: string; date?: string; payment_method?: string }
  | { destination: "gasto_futuro"; due_date: string };

/** Feedback of the expenses module after the Gasto was registered (same shape as POST /expenses). */
export interface RegisteredBudget {
  month: string;
  category: string;
  budget: string | null;
  spent: string;
  remaining: string | null;
  over_budget: boolean;
}

export interface ApproveResult {
  request: ExpenseRequest;
  /** Null for a future expense, or when the feedback could not be computed. */
  budget: RegisteredBudget | null;
}

export const expenseRequestsKeys = {
  all: ["expense-requests"] as const,
  list: (status: RequestStatus | "") => ["expense-requests", "list", status] as const,
  pendingCount: ["expense-requests", "pending-count"] as const,
  categories: ["expense-requests", "categories"] as const,
  budgetCheck: (id: number, category: string, date: string) => ["expense-requests", "budget-check", id, category, date] as const,
};

type Client = Pick<typeof api, "request">;

export function createExpenseRequestsApi(client: Client = api) {
  return {
    /** The household role gets only its own requests; the owner gets all, newest first, optionally by status. */
    list: (status: RequestStatus | "" = "") =>
      client.request<ExpenseRequest[]>(`/expense-requests${status ? `?status=${encodeURIComponent(status)}` : ""}`),
    /** Names of the Gasto categories only (no budgets), so a household user can pick a suggestion. */
    categories: () => client.request<string[]>("/expense-requests/categories"),
    create: (input: NewRequestInput) => client.request<ExpenseRequest>("/expense-requests", { method: "POST", body: input }),
    cancel: (id: number) => client.request<ExpenseRequest>(`/expense-requests/${id}/cancel`, { method: "POST" }),
    budgetCheck: (id: number, category: string, date: string) => {
      const query = new URLSearchParams({ category });
      if (date) query.set("date", date);
      return client.request<BudgetCheck>(`/expense-requests/${id}/budget-check?${query.toString()}`);
    },
    approve: (id: number, input: ApproveInput) => client.request<ApproveResult>(`/expense-requests/${id}/approve`, { method: "POST", body: input }),
    /** Owner only: undoes an approval (deletes the Gasto or the active future expense) and returns the refreshed request. */
    revert: (id: number) => client.request<ExpenseRequest>(`/expense-requests/${id}/revert`, { method: "POST" }),
    reject: (id: number, comment: string) => client.request<ExpenseRequest>(`/expense-requests/${id}/reject`, { method: "POST", body: { comment } }),
  };
}

const defaultApi = createExpenseRequestsApi();

export const listExpenseRequests = defaultApi.list;
export const getRequestCategories = defaultApi.categories;
export const createExpenseRequest = defaultApi.create;
export const cancelExpenseRequest = defaultApi.cancel;
export const getBudgetCheck = defaultApi.budgetCheck;
export const approveExpenseRequest = defaultApi.approve;
export const rejectExpenseRequest = defaultApi.reject;
export const revertExpenseRequest = defaultApi.revert;
