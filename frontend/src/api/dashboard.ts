import { api } from "./client";
import type { PeriodFilingStatus } from "./taxFiling";

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

/** Estimated RESICO ISR of the month plus the filing status of the period. */
export interface DashboardTax {
  /** Decimal fraction, e.g. "0.015". */
  rate: string;
  estimated_isr: string;
  /** Payment state of the filing of this month: `ninguna` when it is not filed yet. */
  filing_status: PeriodFilingStatus;
  /** YYYY-MM of the month before. */
  previous_period: string;
  /** The previous period has issued invoices and no filing, or a filing whose payment is pending. */
  previous_period_pending: boolean;
}

/** Where today sits in the displayed cycle: `day` is 0 before it starts and `days` once it is over. */
export interface DashboardCycle {
  /** YYYY-MM-DD in the configured time zone. */
  today: string;
  day: number;
  days: number;
}

/** An active future expense with what is saved towards it (the savings linked to it). */
export interface FutureExpense {
  id: number;
  name: string;
  /** YYYY-MM-DD */
  due_date: string;
  target: string;
  saved: string;
  remaining: string;
  /** Amount to put aside each cycle to have it on time, rounded up to the cent. */
  suggested_monthly: string;
  cycles_left: number;
}

export interface FutureExpenses {
  items: FutureExpense[];
  target: string;
  saved: string;
  remaining: string;
  suggested_monthly: string;
  /** Gastos futuros savings linked to no item yet, to be assigned from Configuración. */
  free_balance: string;
}

/** A bill due within the next 14 days, or already overdue. `amount` is null for a variable bill. */
export interface UpcomingBill {
  id: number;
  name: string;
  category: string;
  amount: string | null;
  currency: string;
  due_date: string;
  /** Negative when overdue. */
  days_until_due: number;
  overdue: boolean;
}

export type MovementKind = "Ingreso" | "Gasto" | "Ahorro";

/** One of the latest movements of any kind. */
export interface RecentMovement {
  id: number;
  date: string;
  kind: MovementKind;
  description: string;
  category: string;
  amount_mxn: string;
}

/**
 * What the household role receives: only the period and the budget by category (the server drops everything
 * else). The owner's payload extends it.
 */
export interface BudgetDashboard {
  /** YYYY-MM */
  month: string;
  /** First and last day (YYYY-MM-DD, inclusive) of the personal period the month label stands for. */
  period_start: string;
  period_end: string;
  categories: DashboardCategory[];
}

export interface Dashboard extends BudgetDashboard {
  income: string;
  expenses: string;
  /** Ahorro contributed in the month, net of withdrawals. */
  savings: string;
  /** income - expenses - savings. */
  available: string;
  emergency: EmergencyProgress;
  /** Null when the tax settings are incomplete. */
  tax: DashboardTax | null;
  cycle: DashboardCycle;
  future_expenses: FutureExpenses;
  upcoming_bills: UpcomingBill[];
  /** Newest first, at most 8. */
  recent_movements: RecentMovement[];
}

export const dashboardKeys = {
  all: ["dashboard"] as const,
  month: (month: string) => ["dashboard", month] as const,
  /** The household view; an empty month lets the server pick the current cycle. */
  budget: (month: string) => ["dashboard", "budget", month] as const,
};

/**
 * The full payload carries the owner's figures; a reduced one (household role, or a session whose role changed)
 * has only the budget rows. Pages decide by the payload they got, never by assuming fields exist.
 */
export function isFullDashboard(d: BudgetDashboard | Dashboard): d is Dashboard {
  const full = d as Partial<Dashboard>;
  return typeof full.income === "string" && typeof full.expenses === "string" && full.cycle !== undefined && full.future_expenses !== undefined;
}

type Client = Pick<typeof api, "request">;

export function createDashboardApi(client: Client = api) {
  return {
    /** The owner's dashboard. A reduced payload is possible (the role is the server's call): see isFullDashboard. */
    get: (month: string) => client.request<Dashboard | BudgetDashboard>(`/dashboard?month=${encodeURIComponent(month)}`),
    /** The household view. An empty month asks for the current cycle, so the page needs no settings. */
    getBudget: (month: string) => client.request<BudgetDashboard>(`/dashboard?month=${encodeURIComponent(month)}`),
  };
}

const defaultApi = createDashboardApi();

export const getDashboard = defaultApi.get;
export const getBudgetDashboard = defaultApi.getBudget;
