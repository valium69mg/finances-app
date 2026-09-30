import type { DashboardCategory, EmergencyProgress } from "./dashboard";
import type { PeriodFilingStatus } from "./taxFiling";
import { api } from "./client";

/**
 * Month close API (Cierre de mes). A stored close is an immutable snapshot of
 * the figures as of the moment it was generated: editing movements later never
 * changes it. Amounts and percentages travel as decimal strings and are never
 * converted to numbers.
 */

/** Where a positive leftover goes: the emergency fund first, then Inversiones or, while paused, Gastos futuros. */
export interface CloseSuggestion {
  to_emergency_fund: string;
  to_investments: string;
  to_future_expenses: string;
  /** The month is a paused one: the remainder goes to Gastos futuros instead of Inversiones. */
  investments_paused: boolean;
}

/** A budget that deviated more than 20% from the real amount. */
export interface CloseAdjustment {
  category: string;
  kind: "Gasto" | "Ahorro";
  budget: string;
  real: string;
  /** Signed percentage, e.g. "25.5" or "-30". */
  deviation_pct: string;
}

/** A month close. `closed_at` is null on a preview. */
export interface MonthClose {
  /** YYYY-MM */
  period: string;
  /** RFC 3339 instant the snapshot was generated; null on a preview. */
  closed_at: string | null;
  /** Gasto categories against the budgets resolved for the month (same shape as the dashboard). */
  categories: DashboardCategory[];
  income: string;
  expenses: string;
  /** Ahorro contributed in the month, net of withdrawals. */
  savings: string;
  /** income - expenses - savings. */
  available: string;
  /** Emergency fund as of the end of the period. */
  emergency: EmergencyProgress;
  /** Null without a positive leftover. */
  suggestion: CloseSuggestion | null;
  adjustments: CloseAdjustment[];
  /** Filing payment state of the period when the close was computed; null when it was not. */
  tax_filing_status: PeriodFilingStatus | null;
}

/** Preview of a period: computed now, never stored, plus the stored close of the period if any. */
export interface ClosePreview {
  preview: MonthClose;
  existing: MonthClose | null;
}

export const monthCloseKeys = {
  all: ["month-close"] as const,
  preview: (period: string) => ["month-close", "preview", period] as const,
  list: ["month-close", "list"] as const,
};

type Client = Pick<typeof api, "request">;

export function createMonthCloseApi(client: Client = api) {
  return {
    /** Without a period the backend previews the previous month. */
    preview: (period = "") => client.request<ClosePreview>(`/month-close/preview${period ? `?period=${encodeURIComponent(period)}` : ""}`),
    create: (period: string) => client.request<MonthClose>("/month-close", { method: "POST", body: { period } }),
    list: () => client.request<MonthClose[]>("/month-close"),
    get: (period: string) => client.request<MonthClose>(`/month-close/${encodeURIComponent(period)}`),
    /** Discards the snapshot so the period can be closed again; movements are never touched. */
    remove: (period: string) => client.request<void>(`/month-close/${encodeURIComponent(period)}`, { method: "DELETE" }),
  };
}

const defaultApi = createMonthCloseApi();

export const previewMonthClose = defaultApi.preview;
export const createMonthClose = defaultApi.create;
export const listMonthCloses = defaultApi.list;
export const getMonthClose = defaultApi.get;
export const deleteMonthClose = defaultApi.remove;
