import type { Page, Route } from "@playwright/test";
import { cycleOf, cycleRange } from "../src/pages/cycle";
import { createBillsMock, type BillsFail, type MockBill } from "./billsMock";
import { createFutureExpensesMock, type FutureExpensesFail, type MockFutureExpense } from "./futureExpensesMock";
import { createInvoicesMock, type InvoicesFail, type MockInvoice } from "./invoicesMock";
import { createMonthCloseMock, type MockClose, type MonthCloseFail } from "./monthCloseMock";
import { createTaxFilingMock, previousMonthOf, type MockFiling, type TaxFilingFail } from "./taxFilingMock";

export type { BillsFail, MockBill, MockOccurrence } from "./billsMock";
export type { FutureExpensesFail, MockFutureExpense } from "./futureExpensesMock";
export type { MockDocument, MockInvoice, MockUpload } from "./invoicesMock";
export type { MockClose, MonthCloseFail } from "./monthCloseMock";
export type { MockFiling, TaxFilingFail } from "./taxFilingMock";

export const API_ORIGIN = "http://localhost:8080";

export const MODULE_ROUTES = [
  "/",
  "/ingresos",
  "/gastos",
  "/ahorros",
  "/facturas",
  "/declaracion",
  "/declaraciones-presentadas",
  "/cierre-de-mes",
  "/pagos-recurrentes",
  "/sistema",
  "/configuracion",
];

/** Body of GET /system/status, as the backend sends it. */
export interface MockSystemStatus {
  cpu_percent: number;
  memory: { total_bytes: number; used_bytes: number; available_bytes: number; used_percent: number };
  swap: { total_bytes: number; used_bytes: number } | null;
  disk: { total_bytes: number; used_bytes: number; used_percent: number; path_monitored: boolean } | null;
  uptime_seconds: number;
  load_average: { one: number; five: number; fifteen: number };
  disk_alert_percent: number;
  reminders_enabled: boolean;
  sampled_at: string;
}

const GB = 1024 ** 3;

/** A healthy 2 GB VM with a 50 GB data volume; override any field per test. */
export function systemStatus(over: Partial<MockSystemStatus> = {}): MockSystemStatus {
  return {
    cpu_percent: 12.5,
    memory: { total_bytes: 2 * GB, used_bytes: 0.5 * GB, available_bytes: 1.5 * GB, used_percent: 25 },
    swap: { total_bytes: 1 * GB, used_bytes: 0.1 * GB },
    disk: { total_bytes: 50 * GB, used_bytes: 10 * GB, used_percent: 20, path_monitored: true },
    uptime_seconds: 90061,
    load_average: { one: 0.52, five: 0.58, fifteen: 0.59 },
    disk_alert_percent: 80,
    reminders_enabled: true,
    sampled_at: "2026-09-30T18:00:05Z",
    ...over,
  };
}

export type IdentifyMode =
  | "password_required"
  | "verification_sent"
  | "invalid_email"
  | "rate_limited"
  | "network_error"
  | "server_error";
export type LoginMode = "ok" | "unauthorized" | "rate_limited" | "network_error";

export interface RecordedRequest {
  path: string;
  body: unknown;
}

export interface RecordedWrite {
  method: string;
  path: string;
  body: unknown;
}

const CORS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers": "authorization, content-type",
  "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
};

/** PUT route -> key of the aggregate GET /settings payload it replaces. */
const SETTINGS_SECTIONS: Record<string, string> = {
  "/settings/general": "general",
  "/settings/categories": "categories",
  "/settings/clients": "clients",
  "/settings/instruments": "instruments",
  "/settings/brackets": "brackets",
  "/settings/payment-methods": "payment_methods",
  "/settings/issuer": "issuer",
  "/settings/investment-pause": "investment_pause",
};

export const SETTINGS_FIXTURE = {
  general: {
    salary_usd: "2500.00",
    fx_rate_applied: "17.50",
    morse_fee_rate: "0.01",
    emergency_months: "6",
    extra_income_estimate_mxn: "5000.00",
    budget_includes_extra_income: false,
    extra_income_split: { ahorro: "0.5", gasto: "0.5" },
    investment_allocation: [
      { key: "VOO", value: "0.6" },
      { key: "BTC", value: "0.4" },
    ],
    cycle_start_day: 0,
  },
  categories: [
    { name: "Renta", kind: "fixed", budget: "12000.50", includes: "Renta y mantenimiento", keywords: ["renta", "alquiler"] },
    { name: "Comida", kind: "variable", budget: "6000", includes: "", keywords: [] },
    { name: "Inversiones", kind: "savings", budget: null, includes: "", keywords: [] },
    { name: "Servicios", kind: "Gasto", budget: "2000", includes: "", keywords: ["megacable", "luz"] },
    { name: "Suscripciones", kind: "Gasto", budget: null, includes: "", keywords: ["netflix"] },
    { name: "Sueldo", kind: "Ingreso", budget: null, includes: "", keywords: ["sueldo"] },
    { name: "Contrato extra", kind: "Ingreso", budget: null, includes: "", keywords: ["contrato"] },
    { name: "Fondo de emergencia", kind: "Ahorro", budget: null, includes: "", keywords: ["emergencia"] },
    { name: "Inversión ETF", kind: "Ahorro", budget: null, includes: "", keywords: ["etf"] },
  ],
  clients: [
    {
      id: "usa",
      name: "Acme Inc.",
      type: "extranjero",
      currency: "USD",
      iva_rate: "0",
      rfc: "XEXX010101000",
      regimen: "616",
      uso_cfdi: "S01",
      ret_isr_rate: "0",
      ret_iva_rate: "0",
      concepto: "Servicios de software",
      clave_prod_serv: "81111500",
      clave_unidad: "E48",
      address: "",
      tax_residence: "US",
      contract: "",
      real_payer: "",
    },
    {
      id: "b",
      name: "Público en general",
      type: "nacional",
      currency: "MXN",
      iva_rate: "0.16",
      rfc: "XAXX010101000",
      regimen: "616",
      uso_cfdi: "S01",
      ret_isr_rate: "0",
      ret_iva_rate: "0",
      concepto: "Servicios de consultoría",
      clave_prod_serv: "80101500",
      clave_unidad: "E48",
      address: "",
      tax_residence: "MX",
      contract: "",
      real_payer: "Empresa pagadora",
    },
  ],
  instruments: {
    instruments: [
      { id: "i1", name: "VOO", type: "ETF", platform: "GBM" },
      { id: "i2", name: "CETES", type: "Renta fija", platform: "Cetesdirecto" },
    ],
    by_category: { Inversiones: "i1", "Inversión ETF": "i1", "Fondo de emergencia": "i2" },
  },
  brackets: [
    { upper: "25000.00", rate: "0.0100" },
    { upper: "50000.00", rate: "0.0110" },
  ],
  payment_methods: ["Efectivo", "Tarjeta", "Transferencia"],
  issuer: null,
  investment_pause: {
    months: ["2026-10", "2026-11"],
    normal_budget: "5000",
    resume_month: "2027-02",
    future_expenses_plan: { "2026-10": "12000", "2026-11": "12000" },
    note: "",
  },
};

function json(route: Route, status: number, body?: unknown) {
  return route.fulfill({
    status,
    headers: { ...CORS, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** Mocks the whole API surface so tests never need the backend. */
export async function mockApi(
  page: Page,
  opts: { login?: LoginMode; identify?: IdentifyMode; settingsFail?: string; expensesFail?: string; expenses?: MockExpense[]; incomeFail?: string; income?: MockIncome[]; savingsFail?: string; savingsListFail?: boolean; portfolioFail?: boolean; savings?: MockSaving[]; valuations?: MockValuation[]; dashboardFail?: "server" | "incomplete"; dashboardNoTax?: boolean; dashboardFuture?: { items: { id: number; name: string; due_date: string; target: string; saved: string; remaining: string; suggested_monthly: string; cycles_left: number }[]; target: string; saved: string; remaining: string; suggested_monthly: string; free_balance: string }; futureExpenses?: MockFutureExpense[]; futureFreeBalance?: string; futureFail?: FutureExpensesFail; dashboardFiling?: { filing_status: "ninguna" | "pendiente" | "pagada"; previous_period_pending: boolean }; invoices?: MockInvoice[]; invoicesFail?: InvoicesFail; filings?: MockFiling[]; taxFilingFail?: TaxFilingFail; bills?: MockBill[]; billsFail?: BillsFail; closes?: MockClose[]; monthCloseFail?: MonthCloseFail; issuer?: { rfc: string; name: string; regimen: string; postal_code: string; note: string }; cycleStartDay?: number; system?: MockSystemStatus; systemFail?: "unavailable" | "server" } = {},
) {
  const login = opts.login ?? "ok";
  const identify = opts.identify ?? "password_required";
  const settingsFail = opts.settingsFail;
  const expensesFail = opts.expensesFail;
  // In-memory expenses so writes are served back on the next GET.
  const expenses: MockExpense[] = structuredClone(opts.expenses ?? []);
  let nextExpenseId = expenses.reduce((max, e) => Math.max(max, e.id), 0) + 1;
  const incomeFail = opts.incomeFail;
  const income: MockIncome[] = structuredClone(opts.income ?? []);
  let nextIncomeId = income.reduce((max, e) => Math.max(max, e.id), 0) + 1;
  const savingsFail = opts.savingsFail;
  const savings: MockSaving[] = structuredClone(opts.savings ?? []);
  let nextSavingId = savings.reduce((max, e) => Math.max(max, e.id), 0) + 1;
  const valuations: MockValuation[] = structuredClone(opts.valuations ?? []);
  const requests: RecordedRequest[] = [];
  const writes: RecordedWrite[] = [];
  /** Months asked of GET /dashboard, in order. */
  const dashboardMonths: string[] = [];
  /** GET /system/status: change `current` or `fail` between requests to drive a test. */
  const system: { current: MockSystemStatus; fail: "unavailable" | "server" | undefined; calls: number } = {
    current: structuredClone(opts.system ?? systemStatus()),
    fail: opts.systemFail,
    calls: 0,
  };
  // In-memory settings so a saved change is served back on the next GET.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const settings: any = structuredClone(SETTINGS_FIXTURE);
  if (opts.issuer) settings.issuer = opts.issuer;
  if (opts.cycleStartDay !== undefined) settings.general.cycle_start_day = opts.cycleStartDay;
  const invoiceMock = createInvoicesMock(opts.invoices ?? [], opts.invoicesFail, json);
  const taxMock = createTaxFilingMock(opts.filings ?? [], opts.taxFilingFail, {
    invoices: () => invoiceMock.invoices,
    recordExpense: (date, description, amount) => {
      const expense = buildExpense(nextExpenseId++, { date, description, category: "Impuestos", payment_method: "Transferencia", currency: "MXN", amount }, settings);
      expenses.push(expense);
      return expense.id;
    },
    today: todayLocal,
    settings: () => settings,
    json,
  });
  const billsMock = createBillsMock(opts.bills ?? [], opts.billsFail, {
    recordExpense: (body) => {
      const expense = buildExpense(nextExpenseId++, { payment_method: "Débito", ...body }, settings);
      expenses.push(expense);
      return { id: expense.id, amount_mxn: expense.amount_mxn };
    },
    today: todayLocal,
    settings: () => settings,
    json,
  });
  const futureMock = createFutureExpensesMock(opts.futureExpenses ?? [], opts.futureFreeBalance ?? "0", opts.futureFail, {
    recordExpense: (body) => {
      const expense = buildExpense(nextExpenseId++, { payment_method: "Débito", ...body }, settings);
      expenses.push(expense);
      return { id: expense.id, amount_mxn: expense.amount_mxn };
    },
    today: todayLocal,
    settings: () => settings,
    json,
  });
  const monthCloseMock = createMonthCloseMock(opts.closes ?? [], opts.monthCloseFail, {
    // A close is computed from the same in-memory movements as the dashboard.
    figures: (month) => {
      const d = buildDashboard(month, { expenses, income, savings, bills: [] }, settings, false, { filing_status: "ninguna", previous_period_pending: false }, undefined, todayLocal());
      return { categories: d.categories, income: d.income, expenses: d.expenses, savings: d.savings, available: d.available, emergency: d.emergency };
    },
    filingStatus: (month) => taxMock.monthStatus(month).filing_status as "ninguna" | "pendiente" | "pagada",
    today: todayLocal,
    settings: () => settings,
    json,
  });
  await page.route(`${API_ORIGIN}/**`, async (route) => {
    const request = route.request();
    if (request.method() === "OPTIONS") {
      return route.fulfill({ status: 204, headers: CORS });
    }
    const { pathname, searchParams } = new URL(request.url());

    if (await invoiceMock.handle(route, request, pathname, searchParams, settings)) return;
    if (await taxMock.handle(route, request, pathname, searchParams)) return;
    if (await billsMock.handle(route, request, pathname, searchParams)) return;
    if (await futureMock.handle(route, request, pathname)) return;
    if (await monthCloseMock.handle(route, request, pathname, searchParams)) return;

    if (pathname === "/income/infer-category") {
      const d = (searchParams.get("description") ?? "").toLowerCase();
      const hit = Object.entries(INCOME_KEYWORDS).find(([k]) => d.includes(k));
      return json(route, 200, { category: hit ? hit[1] : null });
    }
    if (pathname === "/income" && request.method() === "GET") {
      const month = searchParams.get("month") ?? "";
      const rows = income
        .filter((e) => inCycle(e.date, month, settings))
        .sort((a, b) => (a.date === b.date ? b.id - a.id : a.date < b.date ? 1 : -1));
      return json(route, 200, rows);
    }
    if (pathname === "/income" && request.method() === "POST") {
      const body = request.postDataJSON();
      requests.push({ path: pathname, body });
      if (incomeFail) return json(route, 400, { error: "invalid_income", message: incomeFail });
      const row = buildIncome(nextIncomeId++, body, settings);
      income.push(row);
      return json(route, 201, incomeResult(row, income, true));
    }
    const incomeMatch = /^\/income\/(\d+)$/.exec(pathname);
    if (incomeMatch && (request.method() === "PUT" || request.method() === "DELETE")) {
      const id = Number(incomeMatch[1]);
      const body = request.method() === "PUT" ? request.postDataJSON() : undefined;
      writes.push({ method: request.method(), path: pathname, body });
      const index = income.findIndex((e) => e.id === id);
      if (index < 0) return json(route, 404, { error: "not_found" });
      if (request.method() === "DELETE") {
        income.splice(index, 1);
        return json(route, 204);
      }
      if (incomeFail) return json(route, 400, { error: "invalid_income", message: incomeFail });
      const row = buildIncome(id, body, settings);
      income[index] = row;
      return json(route, 200, incomeResult(row, income, false));
    }

    if (pathname === "/system/status" && request.method() === "GET") {
      system.calls += 1;
      if (system.fail === "unavailable") return json(route, 503, { error: "system_unavailable" });
      if (system.fail === "server") return json(route, 500, { error: "internal_error" });
      return json(route, 200, system.current);
    }

    if (pathname === "/dashboard" && request.method() === "GET") {
      dashboardMonths.push(searchParams.get("month") ?? "");
      if (opts.dashboardFail === "server") return json(route, 500, { error: "internal_error" });
      if (opts.dashboardFail === "incomplete") return json(route, 422, { error: "settings_incomplete", message: "missing required config: emergency_months" });
      const dashboardMonth = searchParams.get("month") ?? "";
      const filing = opts.dashboardFiling ?? taxMock.monthStatus(dashboardMonth);
      return json(route, 200, buildDashboard(dashboardMonth, { expenses, income, savings, bills: billsMock.bills }, settings, !opts.dashboardNoTax, filing, opts.dashboardFuture ?? futureMock.dashboard(), todayLocal()));
    }
    if (pathname === "/savings/portfolio" && request.method() === "GET") {
      if (opts.portfolioFail) return json(route, 500, { error: "internal_error" });
      return json(route, 200, buildPortfolio(savings, valuations, settings));
    }
    if (pathname === "/savings/valuations") {
      if (request.method() === "GET") return json(route, 200, valuations);
      if (request.method() === "POST") {
        const body = request.postDataJSON();
        requests.push({ path: pathname, body });
        if (savingsFail) return json(route, 400, { error: "invalid_valuation", message: savingsFail });
        const valuation: MockValuation = { date: body.date ?? todayLocal(), instrument: body.instrument, value_mxn: body.value_mxn, note: body.note ?? "" };
        valuations.push(valuation);
        return json(route, 201, valuation);
      }
    }
    if (pathname === "/savings/transfers" && request.method() === "POST") {
      const body = request.postDataJSON();
      requests.push({ path: pathname, body });
      if (savingsFail) return json(route, 400, { error: "invalid_saving", message: savingsFail });
      const description = body.description || `Traspaso ${body.from} -> ${body.to}`;
      const category = body.category || "Inversiones";
      const transferId = `00000000-0000-4000-8000-${String(nextSavingId).padStart(12, "0")}`;
      const leg = (instrument: string, amount: string) => ({
        ...buildSaving(nextSavingId++, { instrument, amount, description, category, date: body.date }, settings),
        transfer_id: transferId,
      });
      const out = leg(body.from, `-${body.amount}`);
      const inn = leg(body.to, body.amount);
      savings.push(out, inn);
      return json(route, 201, { out, in: inn });
    }
    if (pathname === "/savings" && request.method() === "GET") {
      if (opts.savingsListFail) return json(route, 500, { error: "internal_error" });
      const month = searchParams.get("month") ?? "";
      const limit = Number(searchParams.get("limit")) || Infinity;
      const rows = savings
        .filter((e) => inCycle(e.date, month, settings))
        .sort((a, b) => (a.date === b.date ? b.id - a.id : a.date < b.date ? 1 : -1))
        .slice(0, limit);
      return json(route, 200, rows);
    }
    if (pathname === "/savings" && request.method() === "POST") {
      const body = request.postDataJSON();
      requests.push({ path: pathname, body });
      if (savingsFail) return json(route, 400, { error: "invalid_saving", message: savingsFail });
      const row = buildSaving(nextSavingId++, body, settings);
      savings.push(row);
      return json(route, 201, row);
    }
    const savingMatch = /^\/savings\/(\d+)$/.exec(pathname);
    if (savingMatch && (request.method() === "PUT" || request.method() === "DELETE")) {
      const id = Number(savingMatch[1]);
      const body = request.method() === "PUT" ? request.postDataJSON() : undefined;
      writes.push({ method: request.method(), path: pathname, body });
      const index = savings.findIndex((e) => e.id === id);
      if (index < 0) return json(route, 404, { error: "not_found" });
      const linked = savings[index].transfer_id;
      if (request.method() === "DELETE") {
        // Deleting a transfer leg removes both legs, like the backend.
        if (linked) savings.splice(0, savings.length, ...savings.filter((s) => s.transfer_id !== linked));
        else savings.splice(index, 1);
        return json(route, 204);
      }
      if (linked) return json(route, 409, { error: "transfer_leg_locked", message: "transfer legs cannot be edited" });
      if (savingsFail) return json(route, 400, { error: "invalid_saving", message: savingsFail });
      const row = buildSaving(id, body, settings, savings[index]);
      savings[index] = row;
      return json(route, 200, row);
    }

    if (pathname === "/expenses/infer-category") {
      const d = (searchParams.get("description") ?? "").toLowerCase();
      const hit = Object.entries(INFER_KEYWORDS).find(([k]) => d.includes(k));
      return json(route, 200, { category: hit ? hit[1] : null });
    }
    if (pathname === "/expenses" && request.method() === "GET") {
      const month = searchParams.get("month") ?? "";
      const rows = expenses
        .filter((e) => inCycle(e.date, month, settings))
        .sort((a, b) => (a.date === b.date ? b.id - a.id : a.date < b.date ? 1 : -1));
      return json(route, 200, rows);
    }
    if (pathname === "/expenses" && request.method() === "POST") {
      const body = request.postDataJSON();
      requests.push({ path: pathname, body });
      if (expensesFail) return json(route, 400, { error: "invalid_expense", message: expensesFail });
      const expense = buildExpense(nextExpenseId++, body, settings);
      expenses.push(expense);
      return json(route, 201, { expense, budget: budgetFor(expense, expenses, settings) });
    }
    const expenseMatch = /^\/expenses\/(\d+)$/.exec(pathname);
    if (expenseMatch && (request.method() === "PUT" || request.method() === "DELETE")) {
      const id = Number(expenseMatch[1]);
      const body = request.method() === "PUT" ? request.postDataJSON() : undefined;
      writes.push({ method: request.method(), path: pathname, body });
      const index = expenses.findIndex((e) => e.id === id);
      if (index < 0) return json(route, 404, { error: "not_found" });
      if (request.method() === "DELETE") {
        expenses.splice(index, 1);
        return json(route, 204);
      }
      if (expensesFail) return json(route, 400, { error: "invalid_expense", message: expensesFail });
      const expense = buildExpense(id, body, settings);
      expenses[index] = expense;
      return json(route, 200, { expense, budget: budgetFor(expense, expenses, settings) });
    }

    if (request.method() === "POST") {
      requests.push({ path: pathname, body: request.postDataJSON() });
    }
    if (request.method() === "PUT" || request.method() === "DELETE") {
      const body = request.method() === "PUT" ? request.postDataJSON() : undefined;
      writes.push({ method: request.method(), path: pathname, body });
      const section = SETTINGS_SECTIONS[pathname];
      if (settingsFail) return json(route, 400, { error: "invalid_settings", message: settingsFail });
      if (section) {
        settings[section] = body;
        return json(route, 204);
      }
      if (pathname === "/settings/investment-pause" && request.method() === "DELETE") {
        settings.investment_pause = null;
        return json(route, 204);
      }
    }
    if (pathname === "/settings") return json(route, 200, settings);
    if (pathname === "/settings/budgets") {
      return json(route, 200, settings.categories.map((c: { name: string; kind: string; budget: string | null }) => ({ name: c.name, kind: c.kind, budget: c.budget })));
    }
    switch (pathname) {
      case "/auth/identify":
        if (identify === "invalid_email") return json(route, 400, { error: "invalid_email" });
        if (identify === "rate_limited") return json(route, 429, { error: "rate_limited" });
        if (identify === "server_error") return json(route, 500, { error: "internal_error" });
        if (identify === "network_error") return route.abort("failed");
        return json(route, 200, { status: identify });
      case "/auth/login":
        if (login === "unauthorized") return json(route, 401, { error: "invalid_credentials" });
        if (login === "rate_limited") return json(route, 429, { error: "rate_limited" });
        if (login === "network_error") return route.abort("failed");
        return json(route, 200, { access_token: "access-1", refresh_token: "refresh-1" });
      case "/auth/me":
        return json(route, 200, { id: "u1", email: "admin@example.com", verified: true });
      case "/auth/refresh":
        return json(route, 200, { access_token: "access-2", refresh_token: "refresh-2" });
      case "/auth/logout":
        return json(route, 204);
      default:
        return json(route, 404, { error: "not_found" });
    }
  });
  return { system, requests, writes, expenses, income, savings, valuations, dashboardMonths, invoices: invoiceMock.invoices, invoiceCalls: invoiceMock.uploads, filings: taxMock.filings, taxWrites: taxMock.writes, bills: billsMock.bills, billWrites: billsMock.writes, futureWrites: futureMock.writes, closes: monthCloseMock.closes, closeWrites: monthCloseMock.writes };
}

/** True when a movement date falls in the personal cycle labelled `month`, using the mock settings' cycle_start_day. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function inCycle(date: string, month: string, settings: any): boolean {
  return cycleOf(date, settings.general.cycle_start_day ?? 0) === month;
}

/** Dashboard computed from the in-memory movements; Gasto categories are the mock's fixed/variable ones. */
function buildDashboard(
  month: string,
  data: { expenses: MockExpense[]; income: MockIncome[]; savings: MockSaving[]; bills: { id: number; name: string; category: string; amount: string | null; currency: string; active: boolean; pending: { due_date: string } }[] },
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  settings: any,
  withTax: boolean,
  filing: { filing_status: string; previous_period_pending: boolean },
  future: { items: unknown[]; target: string; saved: string; remaining: string; suggested_monthly: string; free_balance: string } | undefined,
  today: string,
) {
  const inMonth = <T extends { date: string }>(rows: T[]) => rows.filter((r) => inCycle(r.date, month, settings));
  const sum = (rows: { amount_mxn: string }[]) => rows.reduce((total, r) => total + Number(r.amount_mxn), 0);
  const monthExpenses = inMonth(data.expenses);
  const income = sum(inMonth(data.income));
  const expenses = sum(monthExpenses);
  const savings = sum(inMonth(data.savings));
  const categories = settings.categories
    .filter((c: { kind: string }) => ["Gasto", "fixed", "variable"].includes(c.kind))
    .map((c: { name: string; budget: string | null }) => {
      const spent = sum(monthExpenses.filter((e) => e.category === c.name));
      const budget = c.budget === null ? null : Number(c.budget);
      return {
        category: c.name,
        spent: spent.toFixed(2),
        budget: c.budget,
        remaining: budget === null ? null : (budget - spent).toFixed(2),
        over_budget: budget !== null && spent > budget,
      };
    });
  const rate = resicoRate(income);
  const range = cycleRange(month, settings.general.cycle_start_day ?? 0);
  const dayMs = 86_400_000;
  const utc = (d: string) => Date.parse(`${d}T00:00:00Z`);
  const days = range ? Math.round((utc(range.to) - utc(range.from)) / dayMs) + 1 : 0;
  const day = range ? Math.min(days, Math.max(0, Math.round((utc(today) - utc(range.from)) / dayMs) + 1)) : 0;
  const upcoming = data.bills
    .filter((b) => b.active && Math.round((utc(b.pending.due_date) - utc(today)) / dayMs) <= 14)
    .map((b) => {
      const untilDue = Math.round((utc(b.pending.due_date) - utc(today)) / dayMs);
      return { id: b.id, name: b.name, category: b.category, amount: b.amount, currency: b.currency, due_date: b.pending.due_date, days_until_due: untilDue, overdue: untilDue < 0 };
    })
    .sort((a, b) => (a.due_date < b.due_date ? -1 : a.due_date > b.due_date ? 1 : a.id - b.id));
  const recent = [
    ...data.income.map((m) => ({ ...m, kind: "Ingreso" })),
    ...data.expenses.map((m) => ({ ...m, kind: "Gasto" })),
    ...data.savings.map((m) => ({ ...m, kind: "Ahorro" })),
  ]
    .sort((a, b) => (a.date === b.date ? b.id - a.id : a.date < b.date ? 1 : -1))
    .slice(0, 8)
    .map((m) => ({ id: m.id, date: m.date, kind: m.kind, description: m.description, category: m.category, amount_mxn: m.amount_mxn }));
  return {
    month,
    period_start: range?.from ?? "",
    period_end: range?.to ?? "",
    categories,
    income: income.toFixed(2),
    expenses: expenses.toFixed(2),
    savings: savings.toFixed(2),
    available: (income - expenses - savings).toFixed(2),
    emergency: {
      accumulated: sum(data.savings.filter((s) => s.category === "Fondo de emergencia")).toFixed(2),
      goal: "60000.00",
    },
    cycle: { today, day, days },
    future_expenses: future ?? { items: [], target: "0.00", saved: "0.00", remaining: "0.00", suggested_monthly: "0.00", free_balance: "0.00" },
    upcoming_bills: upcoming,
    recent_movements: recent,
    tax: withTax
      ? {
          rate,
          estimated_isr: (income * Number(rate)).toFixed(5),
          filing_status: filing.filing_status,
          previous_period: previousMonthOf(month),
          previous_period_pending: filing.previous_period_pending,
        }
      : null,
  };
}

export interface MockIncome {
  id: number;
  date: string;
  description: string;
  category: string;
  payment_method: string;
  currency: string;
  amount: string;
  exchange_rate: string | null;
  amount_mxn: string;
}

/** Description keyword -> income category, standing in for the backend's keyword inference. */
const INCOME_KEYWORDS: Record<string, string> = { sueldo: "Sueldo", salario: "Sueldo", contrato: "Contrato extra" };

/** Illustrative RESICO brackets for the mock: up to 25,000 MXN a month pays 1%, above that 1.5%. */
function resicoRate(monthTotal: number) {
  return monthTotal <= 25000 ? "0.01" : "0.015";
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildIncome(id: number, body: any, settings: any): MockIncome {
  const currency = body.currency ?? "MXN";
  const rate: string | null = currency === "USD" ? (body.exchange_rate ?? settings.general.fx_rate_applied) : null;
  const amountMxn = rate ? (Number(body.amount) * Number(rate)).toFixed(2) : body.amount;
  const d = String(body.description ?? "").toLowerCase();
  const inferred = Object.entries(INCOME_KEYWORDS).find(([k]) => d.includes(k))?.[1];
  return {
    id,
    date: body.date ?? todayLocal(),
    description: body.description ?? "",
    category: body.category || inferred || "Sueldo",
    payment_method: body.payment_method ?? "Transferencia",
    currency,
    amount: body.amount,
    exchange_rate: rate,
    amount_mxn: amountMxn,
  };
}

function incomeResult(row: MockIncome, all: MockIncome[], withSplit: boolean) {
  const month = row.date.slice(0, 7);
  const total = all.filter((e) => e.date.startsWith(month)).reduce((sum, e) => sum + Number(e.amount_mxn), 0);
  const previousTotal = total - Number(row.amount_mxn);
  const rate = resicoRate(total);
  const previous = resicoRate(previousTotal);
  return {
    income: row,
    summary: {
      month,
      month_total_mxn: total.toFixed(2),
      resico: {
        rate,
        estimated_isr: (total * Number(rate)).toFixed(5),
        rate_increased: previousTotal > 0 && Number(rate) > Number(previous),
        previous_rate: previousTotal > 0 ? previous : null,
      },
    },
    split:
      withSplit && row.category === "Contrato extra"
        ? {
            sat_reserve: "165",
            emergency_fund: "418",
            investments: "292",
            aguinaldo_vacation: "125",
            goal_reached: false,
            investment_breakdown: [{ instrument: "voo", amount: "292" }],
          }
        : null,
  };
}

export interface MockSaving {
  id: number;
  date: string;
  description: string;
  category: string;
  instrument: string;
  payment_method: string;
  currency: string;
  amount: string;
  exchange_rate: string | null;
  amount_mxn: string;
  /** Shared by both legs of a transfer; null (or absent) for a plain saving. */
  transfer_id?: string | null;
}

export interface MockValuation {
  date: string;
  instrument: string;
  value_mxn: string;
  note: string;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildSaving(id: number, body: any, settings: any, existing?: MockSaving): MockSaving {
  const category = body.category || "Inversiones";
  // Like the backend, an update keeps the stored date, payment method, currency and rate when the body omits them.
  const currency: string = body.currency || existing?.currency || "MXN";
  const keptRate = existing?.currency === "USD" ? existing.exchange_rate : null;
  const rate: string | null = currency === "USD" ? (body.exchange_rate ?? keptRate ?? settings.general.fx_rate_applied) : null;
  return {
    id,
    date: body.date || existing?.date || todayLocal(),
    description: body.description ?? "",
    category,
    instrument: body.instrument || settings.instruments.by_category[category] || "",
    payment_method: body.payment_method || existing?.payment_method || "Transferencia",
    currency,
    amount: body.amount,
    exchange_rate: rate,
    amount_mxn: rate ? (Number(body.amount) * Number(rate)).toFixed(2) : body.amount,
    transfer_id: null,
  };
}

/** Portfolio computed from the in-memory savings and valuations; a later valuation wins. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildPortfolio(savings: MockSaving[], valuations: MockValuation[], settings: any) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rows = settings.instruments.instruments.map((inst: any) => {
    const contributed = savings.filter((s) => s.instrument === inst.id).reduce((sum, s) => sum + Number(s.amount_mxn), 0);
    const latest = valuations.filter((v) => v.instrument === inst.id).reduce<MockValuation | null>((acc, v) => (!acc || v.date >= acc.date ? v : acc), null);
    const value = latest ? Number(latest.value_mxn) : 0;
    const gain = latest ? value - contributed : 0;
    return { inst, contributed, latest, value, gain };
  });
  const totalValue = rows.reduce((sum: number, r: { value: number }) => sum + r.value, 0);
  const totalContributed = rows.reduce((sum: number, r: { contributed: number }) => sum + r.contributed, 0);
  const byType = new Map<string, { contributed: number; value: number }>();
  for (const r of rows) {
    const t = byType.get(r.inst.type) ?? { contributed: 0, value: 0 };
    t.contributed += r.contributed;
    t.value += r.value;
    byType.set(r.inst.type, t);
  }
  const byDestination = new Map<string, number>();
  for (const s of savings) byDestination.set(s.category, (byDestination.get(s.category) ?? 0) + Number(s.amount_mxn));
  return {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    rows: rows.map((r: any) => ({
      id: r.inst.id,
      name: r.inst.name,
      type: r.inst.type,
      platform: r.inst.platform,
      contributed: r.contributed.toFixed(2),
      value: r.value.toFixed(2),
      value_date: r.latest ? r.latest.date : null,
      unvalued: !r.latest,
      gain: r.gain.toFixed(2),
      gain_pct: r.latest && r.contributed !== 0 ? ((r.gain * 100) / r.contributed).toFixed(2) : "0",
      pct_of_total: totalValue !== 0 ? ((r.value * 100) / totalValue).toFixed(2) : "0",
    })),
    total_contributed: totalContributed.toFixed(2),
    total_value: totalValue.toFixed(2),
    by_type: [...byType].map(([type, t]) => ({ type, contributed: t.contributed.toFixed(2), value: t.value.toFixed(2) })),
    by_destination: [...byDestination].map(([category, balance]) => ({ category, balance: balance.toFixed(2) })),
    emergency: { accumulated: (byDestination.get("Fondo de emergencia") ?? 0).toFixed(2), goal: "60000.00" },
  };
}

export interface MockExpense {
  id: number;
  date: string;
  description: string;
  category: string;
  payment_method: string;
  currency: string;
  amount: string;
  exchange_rate: string | null;
  amount_mxn: string;
}

/** Description keyword -> category, standing in for the backend's keyword inference. */
const INFER_KEYWORDS: Record<string, string> = { tacos: "Comida", super: "Comida", uber: "Renta" };

function todayLocal() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildExpense(id: number, body: any, settings: any): MockExpense {
  const currency = body.currency ?? "MXN";
  const rate: string | null = currency === "USD" ? (body.exchange_rate ?? settings.general.fx_rate_applied) : null;
  const amountMxn = rate ? (Number(body.amount) * Number(rate)).toFixed(4) : body.amount;
  const d = String(body.description ?? "").toLowerCase();
  const inferred = Object.entries(INFER_KEYWORDS).find(([k]) => d.includes(k))?.[1];
  return {
    id,
    date: body.date ?? todayLocal(),
    description: body.description ?? "",
    category: body.category || inferred || "Sin categoría",
    payment_method: body.payment_method ?? "Débito",
    currency,
    amount: body.amount,
    exchange_rate: rate,
    amount_mxn: amountMxn,
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function budgetFor(expense: MockExpense, all: MockExpense[], settings: any) {
  const month = expense.date.slice(0, 7);
  const cat = settings.categories.find((c: { name: string }) => c.name === expense.category);
  if (!cat) return null;
  const spent = all
    .filter((e) => e.category === expense.category && e.date.startsWith(month))
    .reduce((sum, e) => sum + Number(e.amount_mxn), 0);
  const budget: string | null = cat.budget;
  const remaining = budget === null ? null : String(Number(budget) - spent);
  return {
    month,
    category: expense.category,
    budget,
    spent: String(spent),
    remaining,
    over_budget: budget !== null && spent > Number(budget),
  };
}

/** Starts the page with a stored session, as if the user had already logged in. */
export async function seedSession(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("access_token", "access-1");
    localStorage.setItem("refresh_token", "refresh-1");
  });
}

/** Runs step one with a verified email so the password step is showing. */
export async function goToPasswordStep(page: Page, email = "admin@example.com") {
  await page.goto("/login");
  await page.getByLabel("Correo electrónico").fill(email);
  await page.getByRole("button", { name: "Continuar" }).click();
  await page.getByLabel("Contraseña", { exact: true }).waitFor();
}
