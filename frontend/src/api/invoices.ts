import { api } from "./client";

/**
 * Invoices API. Amounts and rates travel as decimal strings and are never
 * converted to numbers. The app does not stamp invoices: it prepares the SAT
 * portal checklist, tracks the state and stores the issued CFDI files.
 */

export type InvoiceState = "preparada" | "emitida" | "cancelada";
export type DocumentKind = "xml" | "pdf";
export type Periodicity = "mensual" | "quincenal";

export interface Invoice {
  id: number;
  client_id: string;
  /** YYYY-MM-DD */
  collection_date: string;
  /** YYYY-MM */
  period: string;
  currency: string;
  /** Decimal string, or null for MXN invoices. */
  exchange_rate: string | null;
  subtotal: string;
  subtotal_mxn: string;
  iva: string;
  isr_withheld: string;
  iva_withheld: string;
  total: string;
  expected_deposit_mxn: string;
  state: InvoiceState;
  /** Fiscal UUID once issued. */
  uuid: string | null;
  movement_id: number | null;
  declaration_period: string | null;
  created_at: string;
}

export interface InvoiceDocument {
  id: number;
  kind: DocumentKind;
  name: string;
  content_type: string;
  size: number;
  sha256: string;
  uploaded_at: string;
}

/** Non-blocking finding returned next to a successful result. */
export interface InvoiceWarning {
  code: "possible_duplicate" | "total_mismatch" | "subtotal_mismatch" | "currency_mismatch" | "period_already_filed" | "amounts_from_xml" | (string & {});
  message: string;
  /** possible_duplicate: the existing invoices. */
  invoice_ids?: number[];
  /** Mismatch warnings: the prepared invoice value and the XML value. */
  expected?: string;
  actual?: string;
  /** amounts_from_xml: the stored amounts the XML replaced. */
  changes?: { field: "subtotal" | "iva" | "isr_withheld" | "iva_withheld" | "total" | (string & {}); from: string; to: string }[];
}

export interface ChecklistParty {
  rfc: string;
  name: string;
  regimen: string;
  postal_code: string;
  uso_cfdi?: string;
  /** Real payer of a public-in-general client; not part of the CFDI. */
  internal_note?: string;
}

export interface Checklist {
  issuer: ChecklistParty;
  receiver: ChecklistParty;
  voucher: {
    type: string;
    currency: string;
    exchange_rate: string | null;
    payment_form: string;
    payment_method: string;
    export: boolean;
    global: { periodicity: Periodicity; code: string; months: string; year: string } | null;
  };
  concept: { prod_serv_key: string; unit_key: string; description: string; quantity: number; unit_value: string };
  taxes: { iva_included: boolean; iva: string; isr_withheld: string; iva_withheld: string };
  totals: { currency: string; subtotal: string; total: string; expected_deposit_mxn: string };
  /** YYYY-MM */
  period: string;
  /** YYYY-MM-DD */
  due_date: string;
  /** Settings keys that are empty and needed, e.g. "issuer.rfc". */
  missing_config: string[];
  /** Items to confirm with the accountant. */
  to_confirm: string[];
}

export interface InvoiceDetail {
  invoice: Invoice;
  documents: InvoiceDocument[];
  checklist: Checklist;
  warnings: InvoiceWarning[];
}

/** Client USA takes `subtotal` and `exchange_rate`; every other client takes `amount`, the net amount received (IVA included, retentions already taken off). */
export interface PrepareInput {
  client_id: string;
  date?: string;
  subtotal?: string;
  amount?: string;
  exchange_rate?: string;
  periodicity?: Periodicity;
  movement_id?: number;
}

export interface IssueInput {
  xml?: File | null;
  pdf?: File | null;
  uuid?: string;
}

export interface UploadResult {
  document: InvoiceDocument;
  warnings: InvoiceWarning[];
}

export interface InvoiceFilter {
  /** YYYY-MM; empty for every period. */
  period?: string;
  state?: InvoiceState | "";
}

export const invoiceKeys = {
  all: ["invoices"] as const,
  list: (filter: InvoiceFilter) => ["invoices", "list", filter.period ?? "", filter.state ?? ""] as const,
  detail: (id: number, periodicity: Periodicity) => ["invoices", "detail", id, periodicity] as const,
};

type Client = Pick<typeof api, "request">;

export function createInvoicesApi(client: Client = api) {
  return {
    prepare: (input: PrepareInput) => client.request<InvoiceDetail>("/invoices", { method: "POST", body: input }),
    list: (filter: InvoiceFilter = {}) => {
      const q = new URLSearchParams();
      if (filter.period) q.set("period", filter.period);
      if (filter.state) q.set("state", filter.state);
      const qs = q.toString();
      return client.request<Invoice[]>(`/invoices${qs ? `?${qs}` : ""}`);
    },
    get: (id: number, periodicity?: Periodicity) =>
      client.request<InvoiceDetail>(`/invoices/${id}${periodicity ? `?periodicity=${periodicity}` : ""}`),
    /** Marks the invoice as issued from the CFDI XML or a manual UUID; sent as multipart. */
    issue: (id: number, input: IssueInput) => {
      const form = new FormData();
      if (input.xml) form.append("xml", input.xml);
      if (input.pdf) form.append("pdf", input.pdf);
      if (input.uuid) form.append("uuid", input.uuid);
      return client.request<InvoiceDetail>(`/invoices/${id}/issue`, { method: "POST", body: form });
    },
    /** Attaches or replaces the XML or PDF of an issued invoice. */
    attach: (id: number, kind: DocumentKind, file: File) => {
      const form = new FormData();
      form.append("kind", kind);
      form.append("file", file);
      return client.request<UploadResult>(`/invoices/${id}/documents`, { method: "POST", body: form });
    },
    /** Downloads a stored document through the authenticated API. */
    download: (id: number, docId: number) =>
      client.request<Blob>(`/invoices/${id}/documents/${docId}`, { blob: true }),
    /** Replaces the amounts of an issued, undeclared invoice with the ones in its stored XML. */
    resync: (id: number) => client.request<InvoiceDetail>(`/invoices/${id}/resync`, { method: "POST" }),
    cancel: (id: number) => client.request<Invoice>(`/invoices/${id}/cancel`, { method: "POST" }),
  };
}

const defaultApi = createInvoicesApi();

export const prepareInvoice = defaultApi.prepare;
export const listInvoices = defaultApi.list;
export const getInvoice = defaultApi.get;
export const issueInvoice = defaultApi.issue;
export const attachInvoiceDocument = defaultApi.attach;
export const downloadInvoiceDocument = defaultApi.download;
export const cancelInvoice = defaultApi.cancel;
export const resyncInvoice = defaultApi.resync;
