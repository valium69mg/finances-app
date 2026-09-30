import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { dashboardKeys } from "../../api/dashboard";
import type { Checklist, Invoice, InvoiceDetail as Detail } from "../../api/invoices";
import { invoiceKeys } from "../../api/invoices";
import { taxFilingKeys } from "../../api/taxFiling";
import { SETTINGS } from "./fixtures";
import { InvoiceDetail } from "./InvoiceDetail";
import { IssuePanel } from "./IssuePanel";

const api = vi.hoisted(() => ({ getInvoice: vi.fn(), issueInvoice: vi.fn(), cancelInvoice: vi.fn() }));
vi.mock("../../api/invoices", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/invoices")>()), ...api }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const UUID = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B";

const checklist: Checklist = {
  issuer: { rfc: "AAA010101AAA", name: "Juan", regimen: "626", postal_code: "64000" },
  receiver: { rfc: "XEXX010101000", name: "Acme", regimen: "616", postal_code: "64000", uso_cfdi: "S01" },
  voucher: { type: "I", currency: "USD", exchange_rate: "17.50", payment_form: "03", payment_method: "PUE", export: true, global: null },
  concept: { prod_serv_key: "81111500", unit_key: "E48", description: "Servicios", quantity: 1, unit_value: "2500.00" },
  taxes: { iva_included: false, iva: "0.00" },
  totals: { currency: "USD", subtotal: "2500.00", total: "2500.00", expected_deposit_mxn: "43750.00" },
  period: "2026-10",
  due_date: "2026-11-17",
  missing_config: [],
  to_confirm: [],
};

const invoice = (over: Partial<Invoice> = {}): Invoice => ({
  id: 7,
  client_id: "usa",
  collection_date: "2026-10-15",
  period: "2026-10",
  currency: "USD",
  exchange_rate: "17.50",
  subtotal: "2500.00",
  subtotal_mxn: "43750.00",
  iva: "0.00",
  isr_withheld: "0.00",
  iva_withheld: "0.00",
  total: "2500.00",
  expected_deposit_mxn: "43750.00",
  state: "emitida",
  uuid: UUID,
  movement_id: null,
  declaration_period: null,
  created_at: "2026-10-15T12:00:00Z",
  ...over,
});

const detail = (inv: Invoice): Detail => ({ invoice: inv, documents: [], checklist, warnings: [] });

/** A client holding one query per key the tax filing views use, so invalidation is observable. */
function seededClient() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const keys = {
    invoices: invoiceKeys.list({}),
    preview: taxFilingKeys.preview("2026-10", ""),
    pending: taxFilingKeys.pending,
    unfiled: taxFilingKeys.unfiled,
    records: taxFilingKeys.list({}),
    filing: taxFilingKeys.detail("2026-10"),
    dashboard: dashboardKeys.month("2026-10"),
  };
  for (const key of Object.values(keys)) qc.setQueryData(key, []);
  return { qc, keys };
}

function expectAllInvalidated(qc: QueryClient, keys: Record<string, readonly unknown[]>) {
  for (const [name, key] of Object.entries(keys)) {
    expect(qc.getQueryState(key)?.isInvalidated, name).toBe(true);
  }
}

describe("issuing an invoice", () => {
  it("invalidates the invoices, tax filing views and the dashboard", async () => {
    api.issueInvoice.mockResolvedValue(detail(invoice()));
    const { qc, keys } = seededClient();
    render(
      <QueryClientProvider client={qc}>
        <IssuePanel invoiceId={7} onIssued={() => {}} />
      </QueryClientProvider>,
    );
    fireEvent.change(screen.getByLabelText("UUID manual"), { target: { value: UUID } });
    fireEvent.click(screen.getByRole("button", { name: "Marcar como emitida" }));
    await waitFor(() => expect(api.issueInvoice).toHaveBeenCalled());
    await waitFor(() => expectAllInvalidated(qc, keys));
  });
});

describe("cancelling an invoice", () => {
  function renderDetail(inv: Invoice) {
    api.getInvoice.mockResolvedValue(detail(inv));
    const { qc, keys } = seededClient();
    render(
      <QueryClientProvider client={qc}>
        <InvoiceDetail invoiceId={inv.id} clients={SETTINGS.clients} initialPeriodicity="mensual" warnings={[]} onWarnings={() => {}} onClose={() => {}} />
      </QueryClientProvider>,
    );
    return { qc, keys };
  }

  it("invalidates the invoices, tax filing views and the dashboard", async () => {
    api.cancelInvoice.mockResolvedValue(invoice({ state: "cancelada" }));
    const { qc, keys } = renderDetail(invoice());
    fireEvent.click(await screen.findByRole("button", { name: "Cancelar factura" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, cancelar factura" }));
    await waitFor(() => expect(api.cancelInvoice).toHaveBeenCalledWith(7));
    await waitFor(() => expectAllInvalidated(qc, keys));
  });

  it("disables Cancel for an invoice included in a filing and explains why", async () => {
    renderDetail(invoice({ declaration_period: "2026-10" }));
    const cancel = await screen.findByRole("button", { name: "Cancelar factura" });
    expect(cancel).toBeDisabled();
    expect(screen.getByText(/No se puede cancelar: esta factura está incluida en la declaración de octubre de 2026/)).toBeInTheDocument();
    expect(cancel).toHaveAccessibleDescription(/incluida en la declaración de octubre de 2026/);
    fireEvent.click(cancel);
    expect(api.cancelInvoice).not.toHaveBeenCalled();
  });

  it("shows the message when the server refuses a declared invoice", async () => {
    api.cancelInvoice.mockRejectedValue(new ApiError(409, "invoice_declared", "invoice is included in a filed tax declaration"));
    renderDetail(invoice());
    fireEvent.click(await screen.findByRole("button", { name: "Cancelar factura" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, cancelar factura" }));
    expect(await screen.findByText(/está incluida en una declaración registrada y no se puede cancelar/)).toBeInTheDocument();
  });
});
