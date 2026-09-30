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
  opts: { login?: LoginMode; identify?: IdentifyMode; settingsFail?: string } = {},
) {
  const login = opts.login ?? "ok";
  const identify = opts.identify ?? "password_required";
  const settingsFail = opts.settingsFail;
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
    const { pathname } = new URL(request.url());
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
  return { requests, writes };
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
