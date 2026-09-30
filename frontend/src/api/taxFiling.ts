import type { InvoiceState } from "./invoices";
import { api } from "./client";

/**
 * Tax filing API (Declaración and Declaraciones presentadas). Amounts and rates
 * travel as decimal strings and are never converted to numbers. The app does
 * not file anything with the SAT: it computes the monthly RESICO declaration,
 * records the filing the user made in the SAT portal and its payment.
 */

/** Payment state of a registered filing. */
export type FilingStatus = "pendiente" | "pagada";
/** Payment state of a period: `ninguna` when it has no registered filing. */
export type PeriodFilingStatus = "ninguna" | FilingStatus;

/** Invoice included in a declaration. */
export interface TaxInvoiceRef {
  id: number;
  client_id: string;
  /** YYYY-MM-DD */
  collection_date: string;
  currency: string;
  subtotal_mxn: string;
  state: InvoiceState;
  uuid: string | null;
}

/** Non-blocking notice about the period. */
export interface TaxWarning {
  code: "prepared_invoices" | "already_filed" | (string & {});
  message: string;
  /** prepared_invoices: the invoices still in state preparada. */
  invoice_ids?: number[];
}

export interface FilingPayment {
  /** YYYY-MM-DD */
  date: string;
  isr_paid: string;
  iva_paid: string;
  total_paid: string;
}

export interface Filing {
  /** YYYY-MM */
  period: string;
  /** YYYY-MM-DD */
  filing_date: string;
  /** YYYY-MM-DD, the 17th of the following month. */
  due_date: string;
  income_collected: string;
  /** Decimal fraction, e.g. "0.015". */
  isr_rate: string;
  isr_accrued: string;
  isr_withheld: string;
  /** ISR to pay: accrued minus withheld, never below zero. */
  isr_due: string;
  iva_transferred: string;
  iva_withheld: string;
  iva_acreditable: string;
  /** IVA balance; negative means in favor. */
  iva_due: string;
  /** ISR due plus the IVA due when it is not in favor. */
  total_to_pay: string;
  folio: string;
  status: FilingStatus;
  payment: FilingPayment | null;
  /** Movement id of the Impuestos expense recorded for the payment, if the user chose to. */
  expense_movement_id: number | null;
  invoice_ids: number[];
  created_at: string;
}

export interface FilingDetail extends Filing {
  invoices: TaxInvoiceRef[];
}

/** Computed declaration of a period; nothing is stored. `filing` is set when the period is already filed. */
export interface TaxPreview {
  period: string;
  due_date: string;
  income_collected: string;
  isr_rate: string;
  isr_accrued: string;
  isr_withheld: string;
  isr_due: string;
  iva_transferred: string;
  iva_withheld: string;
  iva_acreditable: string;
  iva_due: string;
  total_to_pay: string;
  /** Income taxed at 0% (export of services, client USA). */
  export_base: string;
  invoices: TaxInvoiceRef[];
  warnings: TaxWarning[];
  filing: Filing | null;
}

export interface PendingPeriod {
  period: string;
  due_date: string;
  overdue: boolean;
}

/** Payment of a filing. Without `date` the backend uses the filing date (register) or today (later). */
export interface PaymentInput {
  date?: string;
  isr_paid: string;
  iva_paid: string;
  /** Also record the total as an Impuestos expense. Never done unless true. */
  record_expense?: boolean;
}

export interface RegisterInput {
  period: string;
  /** YYYY-MM-DD; today when omitted. */
  filing_date?: string;
  folio?: string;
  iva_acreditable?: string;
  /** Omit to register the filing with its payment pending. */
  payment?: PaymentInput;
}

export interface FilingResult {
  filing: Filing;
  warnings: TaxWarning[];
}

export interface FilingFilter {
  /** Four-digit year; empty for every year. */
  year?: string;
  status?: FilingStatus | "";
}

export const taxFilingKeys = {
  all: ["tax-filing"] as const,
  preview: (period: string, ivaAcreditable: string) => ["tax-filing", "preview", period, ivaAcreditable] as const,
  list: (filter: FilingFilter) => ["tax-filing", "list", filter.year ?? "", filter.status ?? ""] as const,
  pending: ["tax-filing", "pending"] as const,
  detail: (period: string) => ["tax-filing", "detail", period] as const,
};

type Client = Pick<typeof api, "request">;

export function createTaxFilingApi(client: Client = api) {
  return {
    preview: (period: string, ivaAcreditable = "") => {
      const q = new URLSearchParams({ period });
      if (ivaAcreditable) q.set("iva_acreditable", ivaAcreditable);
      return client.request<TaxPreview>(`/tax-filing/preview?${q.toString()}`);
    },
    register: (input: RegisterInput) => client.request<FilingResult>("/tax-filing", { method: "POST", body: input }),
    list: (filter: FilingFilter = {}) => {
      const q = new URLSearchParams();
      if (filter.year) q.set("year", filter.year);
      if (filter.status) q.set("status", filter.status);
      const qs = q.toString();
      return client.request<Filing[]>(`/tax-filing${qs ? `?${qs}` : ""}`);
    },
    get: (period: string) => client.request<FilingDetail>(`/tax-filing/${encodeURIComponent(period)}`),
    pay: (period: string, input: PaymentInput) =>
      client.request<FilingResult>(`/tax-filing/${encodeURIComponent(period)}/payment`, { method: "POST", body: input }),
    pending: () => client.request<PendingPeriod[]>("/tax-filing/pending-periods"),
    /** Deletes a filing whose payment is still pending (a paid one is refused). */
    remove: (period: string) => client.request<void>(`/tax-filing/${encodeURIComponent(period)}`, { method: "DELETE" }),
  };
}

const defaultApi = createTaxFilingApi();

export const previewTaxFiling = defaultApi.preview;
export const registerTaxFiling = defaultApi.register;
export const listTaxFilings = defaultApi.list;
export const getTaxFiling = defaultApi.get;
export const payTaxFiling = defaultApi.pay;
export const listPendingPeriods = defaultApi.pending;
export const deleteTaxFiling = defaultApi.remove;
