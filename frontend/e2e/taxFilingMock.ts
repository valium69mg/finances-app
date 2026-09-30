import type { Request, Route } from "@playwright/test";
import type { MockInvoice } from "./invoicesMock";

/** In-memory stand-in for the /tax-filing endpoints, used by helpers.ts. */

export interface MockFiling {
  period: string;
  filing_date: string;
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
  folio: string;
  status: "pendiente" | "pagada";
  payment: { date: string; isr_paid: string; iva_paid: string; total_paid: string } | null;
  expense_movement_id: number | null;
  invoice_ids: number[];
  created_at: string;
}

export type TaxFilingFail = "preview" | "incomplete" | "list" | "pending" | "register" | "pay";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Settings = any;
type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

export interface TaxFilingDeps {
  invoices: () => MockInvoice[];
  /** Adds an Impuestos expense to the shared in-memory expenses and returns its id. */
  recordExpense: (date: string, description: string, amount: string) => number;
  today: () => string;
  settings: () => Settings;
  json: Json;
}

const money = (n: number) => n.toFixed(2);
const cents = (n: number) => Math.round(n * 100) / 100;

export function dueDateOf(period: string) {
  const [y, m] = period.split("-").map(Number);
  return m === 12 ? `${y + 1}-01-17` : `${y}-${String(m + 1).padStart(2, "0")}-17`;
}

export function previousMonthOf(month: string) {
  const [y, m] = month.split("-").map(Number);
  return m === 1 ? `${y - 1}-12` : `${y}-${String(m - 1).padStart(2, "0")}`;
}

export function createTaxFilingMock(seed: MockFiling[], fail: TaxFilingFail | undefined, deps: TaxFilingDeps) {
  const filings: MockFiling[] = structuredClone(seed);
  /** Every write the page sent: method, path and JSON body. */
  const writes: { method: string; path: string; body: unknown }[] = [];

  const toMxn = (inv: MockInvoice, value: string) => (inv.currency === "USD" ? Number(value) * Number(inv.exchange_rate ?? 1) : Number(value));
  const invoiceRef = (inv: MockInvoice) => ({
    id: inv.id,
    client_id: inv.client_id,
    collection_date: inv.collection_date,
    currency: inv.currency,
    subtotal_mxn: inv.subtotal_mxn,
    state: inv.state,
    uuid: inv.uuid,
  });

  function compute(period: string, ivaAcreditable: string) {
    const settings = deps.settings();
    const all = deps.invoices().filter((i) => i.period === period);
    const included = all.filter((i) => i.state === "emitida").sort((a, b) => a.id - b.id);
    const prepared = all.filter((i) => i.state === "preparada").map((i) => i.id);
    const income = cents(included.reduce((s, i) => s + Number(i.subtotal_mxn), 0));
    const bracket = income > 0 ? settings.brackets.find((b: { upper: string }) => income <= Number(b.upper)) ?? settings.brackets.at(-1) : settings.brackets[0];
    const rate: string = bracket.rate;
    const accrued = cents(income * Number(rate));
    const withheld = cents(included.reduce((s, i) => s + toMxn(i, i.isr_withheld), 0));
    const ivaTransferred = cents(included.reduce((s, i) => s + toMxn(i, i.iva), 0));
    const ivaWithheld = cents(included.reduce((s, i) => s + toMxn(i, i.iva_withheld), 0));
    const creditable = Number(ivaAcreditable || "0");
    const isrDue = Math.max(0, accrued - withheld);
    const ivaDue = ivaTransferred - ivaWithheld - creditable;
    return {
      included,
      prepared,
      amounts: {
        period,
        due_date: dueDateOf(period),
        income_collected: money(income),
        isr_rate: rate,
        isr_accrued: money(accrued),
        isr_withheld: money(withheld),
        isr_due: money(isrDue),
        iva_transferred: money(ivaTransferred),
        iva_withheld: money(ivaWithheld),
        iva_acreditable: money(creditable),
        iva_due: money(ivaDue),
        total_to_pay: money(isrDue + Math.max(0, ivaDue)),
      },
      exportBase: money(included.filter((i) => i.client_id === "usa").reduce((s, i) => s + Number(i.subtotal_mxn), 0)),
    };
  }

  const link = (filing: MockFiling) => {
    const ids = new Set(filing.invoice_ids);
    return deps.invoices().filter((i) => ids.has(i.id));
  };

  /** Payment state of a month and whether its previous period needs action, as GET /dashboard reports them. */
  function monthStatus(month: string) {
    const prev = previousMonthOf(month);
    const current = filings.find((f) => f.period === month);
    const previous = filings.find((f) => f.period === prev);
    const previousPending = previous ? previous.status === "pendiente" : deps.invoices().some((i) => i.period === prev && i.state === "emitida");
    return { filing_status: current ? current.status : "ninguna", previous_period_pending: previousPending };
  }

  async function handle(route: Route, request: Request, pathname: string, params: URLSearchParams): Promise<boolean> {
    if (!pathname.startsWith("/tax-filing")) return false;
    const { json } = deps;
    const method = request.method();

    if (pathname === "/tax-filing/preview" && method === "GET") {
      if (fail === "incomplete") return json(route, 422, { error: "settings_incomplete", message: "missing required config: resico_brackets" }).then(() => true);
      if (fail === "preview") return json(route, 500, { error: "internal_error" }).then(() => true);
      const period = params.get("period") ?? "";
      if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(period)) {
        await json(route, 400, { error: "invalid_filing", message: `invalid tax filing input: invalid period "${period}", use YYYY-MM` });
        return true;
      }
      const { included, prepared, amounts, exportBase } = compute(period, params.get("iva_acreditable") ?? "");
      const filed = filings.find((f) => f.period === period) ?? null;
      const warnings: unknown[] = [];
      if (prepared.length) warnings.push({ code: "prepared_invoices", message: "the period includes invoices still in state preparada", invoice_ids: prepared });
      if (filed) warnings.push({ code: "already_filed", message: `the period ${period} was already filed on ${filed.filing_date}` });
      await json(route, 200, { ...amounts, export_base: exportBase, invoices: included.map(invoiceRef), warnings, filing: filed });
      return true;
    }

    if (pathname === "/tax-filing/pending-periods" && method === "GET") {
      if (fail === "pending") return json(route, 500, { error: "internal_error" }).then(() => true);
      const filed = new Set(filings.map((f) => f.period));
      const periods = [...new Set(deps.invoices().filter((i) => i.state === "emitida" && !filed.has(i.period)).map((i) => i.period))].sort();
      const today = deps.today();
      await json(route, 200, periods.map((p) => ({ period: p, due_date: dueDateOf(p), overdue: today > dueDateOf(p) })));
      return true;
    }

    if (pathname === "/tax-filing" && method === "GET") {
      if (fail === "list") return json(route, 500, { error: "internal_error" }).then(() => true);
      const year = params.get("year") ?? "";
      const status = params.get("status") ?? "";
      const rows = filings
        .filter((f) => (!year || f.period.startsWith(`${year}-`)) && (!status || f.status === status))
        .sort((a, b) => (a.period < b.period ? 1 : -1));
      await json(route, 200, rows);
      return true;
    }

    if (pathname === "/tax-filing" && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "register") return json(route, 500, { error: "internal_error" }).then(() => true);
      if (filings.some((f) => f.period === body.period)) {
        await json(route, 409, { error: "already_filed", message: `period already filed: ${body.period}` });
        return true;
      }
      const { included, prepared, amounts } = compute(body.period, body.iva_acreditable ?? "");
      let payment: MockFiling["payment"] = null;
      let expenseId: number | null = null;
      if (body.payment) {
        const paidDate = body.payment.date || body.filing_date || deps.today();
        const total = Number(body.payment.isr_paid) + Number(body.payment.iva_paid);
        payment = { date: paidDate, isr_paid: body.payment.isr_paid, iva_paid: body.payment.iva_paid, total_paid: money(total) };
        if (body.payment.record_expense) expenseId = deps.recordExpense(paidDate, `Pago SAT ISR+IVA periodo ${body.period}`, money(total));
      }
      const filing: MockFiling = {
        ...amounts,
        filing_date: body.filing_date || deps.today(),
        folio: body.folio ?? "",
        status: payment ? "pagada" : "pendiente",
        payment,
        expense_movement_id: expenseId,
        invoice_ids: included.map((i) => i.id),
        created_at: "2026-11-05T12:00:00Z",
      };
      filings.push(filing);
      for (const inv of link(filing)) inv.declaration_period = filing.period;
      const warnings = prepared.length ? [{ code: "prepared_invoices", message: "the period includes invoices still in state preparada", invoice_ids: prepared }] : [];
      await json(route, 201, { filing, warnings });
      return true;
    }

    const m = /^\/tax-filing\/(\d{4}-\d{2})(\/payment)?$/.exec(pathname);
    if (!m) return false;
    const filing = filings.find((f) => f.period === m[1]);

    if (m[2] && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "pay") return json(route, 500, { error: "internal_error" }).then(() => true);
      if (!filing) {
        await json(route, 404, { error: "not_found" });
        return true;
      }
      if (filing.payment) {
        await json(route, 409, { error: "already_paid", message: `filing payment already recorded: ${filing.period}` });
        return true;
      }
      const paidDate = body.date || deps.today();
      const total = Number(body.isr_paid) + Number(body.iva_paid);
      filing.payment = { date: paidDate, isr_paid: body.isr_paid, iva_paid: body.iva_paid, total_paid: money(total) };
      filing.status = "pagada";
      if (body.record_expense) filing.expense_movement_id = deps.recordExpense(paidDate, `Pago SAT ISR+IVA periodo ${filing.period}`, money(total));
      await json(route, 200, { filing, warnings: [] });
      return true;
    }

    if (!m[2] && method === "GET") {
      if (!filing) {
        await json(route, 404, { error: "not_found" });
        return true;
      }
      await json(route, 200, { ...filing, invoices: link(filing).map(invoiceRef) });
      return true;
    }

    if (!m[2] && method === "DELETE") {
      writes.push({ method, path: pathname, body: undefined });
      if (!filing) {
        await json(route, 404, { error: "not_found" });
        return true;
      }
      if (filing.payment) {
        await json(route, 409, { error: "filing_paid", message: "a paid filing cannot be deleted" });
        return true;
      }
      for (const inv of link(filing)) inv.declaration_period = null;
      filings.splice(filings.indexOf(filing), 1);
      await json(route, 204);
      return true;
    }
    return false;
  }

  return { filings, writes, handle, monthStatus };
}
