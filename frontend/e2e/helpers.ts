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

const CORS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers": "authorization, content-type",
  "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
};

function json(route: Route, status: number, body?: unknown) {
  return route.fulfill({
    status,
    headers: { ...CORS, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** Mocks the whole API surface so tests never need the backend. */
export async function mockApi(page: Page, opts: { login?: LoginMode; identify?: IdentifyMode } = {}) {
  const login = opts.login ?? "ok";
  const identify = opts.identify ?? "password_required";
  const requests: RecordedRequest[] = [];
  await page.route(`${API_ORIGIN}/**`, async (route) => {
    const request = route.request();
    if (request.method() === "OPTIONS") {
      return route.fulfill({ status: 204, headers: CORS });
    }
    const { pathname } = new URL(request.url());
    if (request.method() === "POST") {
      requests.push({ path: pathname, body: request.postDataJSON() });
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
  return { requests };
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
