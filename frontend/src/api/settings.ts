import { api } from "./client";

/**
 * Settings API. Money, rates and month counts travel as decimal strings and are
 * never converted to numbers, so no precision is lost.
 */

export interface Weight {
  key: string;
  value: string;
}

export interface General {
  salary_usd: string;
  fx_rate_applied: string;
  morse_fee_rate: string;
  emergency_months: string;
  extra_income_estimate_mxn: string;
  budget_includes_extra_income: boolean;
  extra_income_split: Record<string, string>;
  investment_allocation: Weight[];
  /** 0 = calendar month, 31 = cycle starts on the last day of the previous month. */
  cycle_start_day: number;
}

export interface Category {
  name: string;
  kind: string;
  /** Decimal string, or null when the category has no budget. */
  budget: string | null;
  includes: string;
  keywords: string[];
}

export interface Client {
  id: string;
  name: string;
  type: string;
  currency: string;
  iva_rate: string;
  rfc: string;
  regimen: string;
  uso_cfdi: string;
  ret_isr_rate: string;
  ret_iva_rate: string;
  concepto: string;
  clave_prod_serv: string;
  clave_unidad: string;
  address: string;
  tax_residence: string;
  contract: string;
  real_payer: string;
}

export interface Instrument {
  id: string;
  name: string;
  type: string;
  platform: string;
}

export interface Instruments {
  instruments: Instrument[];
  /** Category name -> instrument id. */
  by_category: Record<string, string>;
}

export interface Bracket {
  upper: string;
  rate: string;
}

export interface Issuer {
  rfc: string;
  name: string;
  regimen: string;
  postal_code: string;
  note: string;
}

export interface Pause {
  /** Paused months as YYYY-MM. */
  months: string[];
  normal_budget: string;
  resume_month: string;
  /** Month (YYYY-MM) -> planned future-expenses amount. */
  future_expenses_plan: Record<string, string>;
  note: string;
}

export interface MonthBudget {
  name: string;
  kind: string;
  budget: string | null;
}

export interface AllSettings {
  general: General;
  categories: Category[];
  clients: Client[];
  instruments: Instruments;
  brackets: Bracket[];
  payment_methods: string[];
  issuer: Issuer | null;
  investment_pause: Pause | null;
}

export const settingsKeys = {
  all: ["settings"] as const,
  budgets: (month: string) => ["settings", "budgets", month] as const,
};

const put = <T>(path: string, body: T) => api.request<void>(`/settings/${path}`, { method: "PUT", body });

export const getSettings = () => api.request<AllSettings>("/settings");
export const getMonthBudgets = (month: string) =>
  api.request<MonthBudget[]>(`/settings/budgets?month=${encodeURIComponent(month)}`);

export const updateGeneral = (v: General) => put("general", v);
export const updateCategories = (v: Category[]) => put("categories", v);
export const updateClients = (v: Client[]) => put("clients", v);
export const updateInstruments = (v: Instruments) => put("instruments", v);
export const updateBrackets = (v: Bracket[]) => put("brackets", v);
export const updatePaymentMethods = (v: string[]) => put("payment-methods", v);
export const updateIssuer = (v: Issuer) => put("issuer", v);
export const updatePause = (v: Pause) => put("investment-pause", v);
export const deletePause = () => api.request<void>("/settings/investment-pause", { method: "DELETE" });
