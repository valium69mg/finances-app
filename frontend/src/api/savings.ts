import { api } from "./client";

/**
 * Savings API. Amounts and percentages travel as decimal strings and are never
 * converted to numbers, so no precision is lost. Valuations are append-only:
 * there is deliberately no update or delete for them.
 */

export interface Saving {
  id: number;
  /** YYYY-MM-DD */
  date: string;
  description: string;
  category: string;
  /** Instrument id from the settings. */
  instrument: string;
  payment_method: string;
  currency: string;
  /** Negative for a withdrawal. */
  amount: string;
  /** Decimal string, or null for MXN savings. */
  exchange_rate: string | null;
  amount_mxn: string;
  /** Shared by the two legs of a transfer; null for any other saving. Legs cannot be edited, and deleting one deletes both. */
  transfer_id: string | null;
}

/** Body for create and update. Only `amount` is required; the backend fills the rest. */
export interface SavingInput {
  date?: string;
  description?: string;
  category?: string;
  instrument?: string;
  payment_method?: string;
  currency?: string;
  amount: string;
  exchange_rate?: string;
}

export interface TransferInput {
  from: string;
  to: string;
  amount: string;
  date?: string;
  description?: string;
  category?: string;
}

export interface TransferResult {
  out: Saving;
  in: Saving;
}

export interface ValuationInput {
  instrument: string;
  value_mxn: string;
  date?: string;
  note?: string;
}

export interface Valuation {
  /** YYYY-MM-DD */
  date: string;
  instrument: string;
  value_mxn: string;
  note: string;
}

export interface PortfolioRow {
  id: string;
  name: string;
  type: string;
  platform: string;
  contributed: string;
  value: string;
  /** YYYY-MM-DD, or null when the instrument was never valued. */
  value_date: string | null;
  unvalued: boolean;
  gain: string;
  /** Already a percentage, e.g. "12.5" means 12.5%. */
  gain_pct: string;
  /** Already a percentage. */
  pct_of_total: string;
}

export interface TypeTotal {
  type: string;
  contributed: string;
  value: string;
}

export interface DestinationTotal {
  category: string;
  balance: string;
}

export interface EmergencyProgress {
  accumulated: string;
  goal: string;
}

export interface Portfolio {
  rows: PortfolioRow[];
  total_contributed: string;
  total_value: string;
  by_type: TypeTotal[];
  by_destination: DestinationTotal[];
  emergency: EmergencyProgress;
}

export const savingsKeys = {
  all: ["savings"] as const,
  list: (month: string) => ["savings", "list", month] as const,
  portfolio: ["savings", "portfolio"] as const,
  valuations: ["savings", "valuations"] as const,
};

type Client = Pick<typeof api, "request">;

export function createSavingsApi(client: Client = api) {
  return {
    list: (month: string, limit = 200) =>
      client.request<Saving[]>(`/savings?month=${encodeURIComponent(month)}&limit=${limit}`),
    create: (input: SavingInput) => client.request<Saving>("/savings", { method: "POST", body: input }),
    update: (id: number, input: SavingInput) => client.request<Saving>(`/savings/${id}`, { method: "PUT", body: input }),
    remove: (id: number) => client.request<void>(`/savings/${id}`, { method: "DELETE" }),
    transfer: (input: TransferInput) => client.request<TransferResult>("/savings/transfers", { method: "POST", body: input }),
    addValuation: (input: ValuationInput) => client.request<Valuation>("/savings/valuations", { method: "POST", body: input }),
    listValuations: () => client.request<Valuation[]>("/savings/valuations"),
    portfolio: () => client.request<Portfolio>("/savings/portfolio"),
  };
}

const defaultApi = createSavingsApi();

export const listSavings = defaultApi.list;
export const createSaving = defaultApi.create;
export const updateSaving = defaultApi.update;
export const deleteSaving = defaultApi.remove;
export const transferSavings = defaultApi.transfer;
export const addValuation = defaultApi.addValuation;
export const listValuations = defaultApi.listValuations;
export const getPortfolio = defaultApi.portfolio;
