import { api } from "./client";

/**
 * Dashboard API (read-only). Amounts and rates travel as decimal strings and
 * are never converted to numbers, so no precision is lost.
 */

/** One Gasto category of the month against its budget. */
export interface DashboardCategory {
  category: string;
  spent: string;
  /** Budget resolved for the month, or null when the category has none. */
  budget: string | null;
  /** budget - spent (negative when exceeded), or null without a budget. */
  remaining: string | null;
  over_budget: boolean;
}

export interface EmergencyProgress {
  accumulated: string;
  goal: string;
}

/** Estimated RESICO ISR of the month. It carries no payment status yet. */
export interface DashboardTax {
  /** Decimal fraction, e.g. "0.015". */
  rate: string;
  estimated_isr: string;
}

export interface Dashboard {
  /** YYYY-MM */
  month: string;
  categories: DashboardCategory[];
  income: string;
  expenses: string;
  /** Ahorro contributed in the month, net of withdrawals. */
  savings: string;
  /** income - expenses - savings. */
  available: string;
  emergency: EmergencyProgress;
  /** Null when the tax settings are incomplete. */
  tax: DashboardTax | null;
}

export const dashboardKeys = {
  all: ["dashboard"] as const,
  month: (month: string) => ["dashboard", month] as const,
};

type Client = Pick<typeof api, "request">;

export function createDashboardApi(client: Client = api) {
  return {
    get: (month: string) => client.request<Dashboard>(`/dashboard?month=${encodeURIComponent(month)}`),
  };
}

const defaultApi = createDashboardApi();

export const getDashboard = defaultApi.get;
