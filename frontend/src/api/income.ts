import { api } from "./client";

/**
 * Income API. Amounts and rates travel as decimal strings and are never
 * converted to numbers, so no precision is lost.
 */

export interface Income {
  id: number;
  /** YYYY-MM-DD */
  date: string;
  description: string;
  category: string;
  payment_method: string;
  currency: string;
  amount: string;
  /** Decimal string, or null for MXN income. */
  exchange_rate: string | null;
  amount_mxn: string;
}

/** Body for create and update. Only `amount` is required; the backend fills the rest. */
export interface IncomeInput {
  date?: string;
  description?: string;
  category?: string;
  payment_method?: string;
  currency?: string;
  amount: string;
  exchange_rate?: string;
}

export interface ResicoEstimate {
  /** Decimal fraction, e.g. "0.015". */
  rate: string;
  estimated_isr: string;
  rate_increased: boolean;
  previous_rate: string | null;
}

export interface IncomeSummary {
  /** YYYY-MM */
  month: string;
  month_total_mxn: string;
  resico: ResicoEstimate | null;
}

export interface InvestmentBreakdownItem {
  instrument: string;
  amount: string;
}

/** Suggested split of an extra contract income. It is never persisted. */
export interface IncomeSplit {
  sat_reserve: string;
  emergency_fund: string;
  investments: string;
  aguinaldo_vacation: string;
  goal_reached: boolean;
  investment_breakdown: InvestmentBreakdownItem[];
}

export interface SaveIncomeResult {
  income: Income;
  summary: IncomeSummary;
  split: IncomeSplit | null;
}

export const incomeKeys = {
  all: ["income"] as const,
  list: (month: string) => ["income", "list", month] as const,
  infer: (description: string) => ["income", "infer", description] as const,
};

type Client = Pick<typeof api, "request">;

export function createIncomeApi(client: Client = api) {
  return {
    list: (month: string, limit = 200) =>
      client.request<Income[]>(`/income?month=${encodeURIComponent(month)}&limit=${limit}`),
    create: (input: IncomeInput) => client.request<SaveIncomeResult>("/income", { method: "POST", body: input }),
    update: (id: number, input: IncomeInput) =>
      client.request<SaveIncomeResult>(`/income/${id}`, { method: "PUT", body: input }),
    remove: (id: number) => client.request<void>(`/income/${id}`, { method: "DELETE" }),
    /** Suggested category for a description, or null when nothing matches. */
    inferCategory: async (description: string) => {
      const res = await client.request<{ category: string | null }>(
        `/income/infer-category?description=${encodeURIComponent(description)}`,
      );
      return res.category;
    },
  };
}

const defaultApi = createIncomeApi();

export const listIncome = defaultApi.list;
export const createIncome = defaultApi.create;
export const updateIncome = defaultApi.update;
export const deleteIncome = defaultApi.remove;
export const inferIncomeCategory = defaultApi.inferCategory;
