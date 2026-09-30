import type { Page, Route } from "@playwright/test";

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
  "/configuracion",
];

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
  },
  categories: [
    { name: "Renta", kind: "fixed", budget: "12000.50", includes: "Renta y mantenimiento", keywords: ["renta", "alquiler"] },
    { name: "Comida", kind: "variable", budget: "6000", includes: "", keywords: [] },
    { name: "Inversiones", kind: "savings", budget: null, includes: "", keywords: [] },
  ],
  clients: [
    {
      id: "c1",
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
  ],
  instruments: {
    instruments: [{ id: "i1", name: "VOO", type: "ETF", platform: "GBM" }],
    by_category: { Inversiones: "i1" },
  },
  brackets: [
    { upper: "25000.00", rate: "0.0100" },
    { upper: "50000.00", rate: "0.0110" },
  ],
  payment_methods: ["Efectivo", "Tarjeta"],
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
  opts: { login?: LoginMode; identify?: IdentifyMode; settingsFail?: string; expensesFail?: string; expenses?: MockExpense[] } = {},
) {
  const login = opts.login ?? "ok";
  const identify = opts.identify ?? "password_required";
  const settingsFail = opts.settingsFail;
  const expensesFail = opts.expensesFail;
  // In-memory expenses so writes are served back on the next GET.
  const expenses: MockExpense[] = structuredClone(opts.expenses ?? []);
  let nextExpenseId = expenses.reduce((max, e) => Math.max(max, e.id), 0) + 1;
  const requests: RecordedRequest[] = [];
  const writes: RecordedWrite[] = [];
  // In-memory settings so a saved change is served back on the next GET.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const settings: any = structuredClone(SETTINGS_FIXTURE);
  await page.route(`${API_ORIGIN}/**`, async (route) => {
    const request = route.request();
    if (request.method() === "OPTIONS") {
      return route.fulfill({ status: 204, headers: CORS });
    }
    const { pathname, searchParams } = new URL(request.url());

    if (pathname === "/expenses/infer-category") {
      const d = (searchParams.get("description") ?? "").toLowerCase();
      const hit = Object.entries(INFER_KEYWORDS).find(([k]) => d.includes(k));
      return json(route, 200, { category: hit ? hit[1] : null });
    }
    if (pathname === "/expenses" && request.method() === "GET") {
      const month = searchParams.get("month") ?? "";
      const rows = expenses
        .filter((e) => e.date.startsWith(month))
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
  return { requests, writes, expenses };
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
