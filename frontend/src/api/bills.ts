import { api } from "./client";

/**
 * Bills & Subscriptions API (Pagos recurrentes). Amounts travel as decimal
 * strings and are never converted to numbers. Only the next occurrence of a
 * bill exists at a time; paying or skipping it generates the following one.
 */

export type Recurrence = "weekly" | "biweekly" | "monthly" | "bimonthly" | "yearly";
export type OccurrenceStatus = "pending" | "paid" | "skipped";

export interface Occurrence {
  id: number;
  /** YYYY-MM-DD */
  due_date: string;
  status: OccurrenceStatus;
  /** YYYY-MM-DD, only for a paid occurrence. */
  paid_on: string | null;
  /** Expense registered by the payment; null when skipped or when that expense was deleted later. */
  expense_movement_id: number | null;
  amount_paid: string | null;
  currency: string | null;
  resolved_at: string | null;
}

export interface Bill {
  id: number;
  name: string;
  /** Default expense category, changeable when paying. */
  category: string;
  /** Fixed amount; null for a variable bill. */
  amount: string | null;
  currency: string;
  recurrence: Recurrence;
  /** YYYY-MM-DD, due date of the pending occurrence. */
  next_due_date: string;
  reminder_lead_days: number;
  active: boolean;
  notes: string;
  created_at: string;
  pending_occurrence: Occurrence | null;
  /** The pending occurrence is past its due date (a bill due today is not overdue). */
  overdue: boolean;
  /** The pending occurrence is not overdue and falls within reminder_lead_days. */
  due_soon: boolean;
  /** Negative when overdue; null without a pending occurrence. */
  days_until_due: number | null;
}

export interface BillDetail extends Bill {
  /** Paid and skipped occurrences, newest due date first. */
  history: Occurrence[];
}

/** Body of create and update. A null amount makes a variable bill. */
export interface BillInput {
  name: string;
  category: string;
  amount: string | null;
  currency: string;
  recurrence: Recurrence;
  next_due_date: string;
  reminder_lead_days: number;
  active: boolean;
  notes: string;
}

/** Payment of the pending occurrence. Every field is optional except `amount` for a variable bill. */
export interface PayInput {
  date?: string;
  amount?: string;
  category?: string;
  description?: string;
}

export interface PaidExpense {
  id: number;
  date: string;
  description: string;
  category: string;
  currency: string;
  amount: string;
  amount_mxn: string;
}

export interface PayResult {
  /** The bill with its next pending occurrence. */
  bill: Bill;
  paid_occurrence: Occurrence;
  expense: PaidExpense;
}

export const billsKeys = {
  all: ["bills"] as const,
  list: (includeInactive: boolean) => ["bills", "list", includeInactive] as const,
  detail: (id: number) => ["bills", "detail", id] as const,
};

type Client = Pick<typeof api, "request">;

export function createBillsApi(client: Client = api) {
  return {
    list: (includeInactive = false) => client.request<Bill[]>(`/bills${includeInactive ? "?include_inactive=true" : ""}`),
    get: (id: number) => client.request<BillDetail>(`/bills/${id}`),
    create: (input: BillInput) => client.request<Bill>("/bills", { method: "POST", body: input }),
    update: (id: number, input: BillInput) => client.request<Bill>(`/bills/${id}`, { method: "PUT", body: input }),
    /** Deactivates the bill: its history and the expenses of past payments stay. */
    deactivate: (id: number) => client.request<void>(`/bills/${id}`, { method: "DELETE" }),
    pay: (id: number, input: PayInput) => client.request<PayResult>(`/bills/${id}/pay`, { method: "POST", body: input }),
    /** Skips the pending occurrence without registering an expense. */
    skip: (id: number) => client.request<Bill>(`/bills/${id}/skip`, { method: "POST" }),
  };
}

const defaultApi = createBillsApi();

export const listBills = defaultApi.list;
export const getBill = defaultApi.get;
export const createBill = defaultApi.create;
export const updateBill = defaultApi.update;
export const deactivateBill = defaultApi.deactivate;
export const payBill = defaultApi.pay;
export const skipBill = defaultApi.skip;
