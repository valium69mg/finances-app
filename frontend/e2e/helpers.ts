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

export type LoginMode = "ok" | "unauthorized" | "pending";

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
export async function mockApi(page: Page, opts: { login?: LoginMode } = {}) {
  const login = opts.login ?? "ok";
  await page.route(`${API_ORIGIN}/**`, async (route) => {
    const request = route.request();
    if (request.method() === "OPTIONS") {
      return route.fulfill({ status: 204, headers: CORS });
    }
    const { pathname } = new URL(request.url());
    switch (pathname) {
      case "/auth/login":
        if (login === "unauthorized") return json(route, 401, { error: "invalid_credentials" });
        if (login === "pending") return json(route, 202, { status: "verification_pending" });
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
}

/** Starts the page with a stored session, as if the user had already logged in. */
export async function seedSession(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("access_token", "access-1");
    localStorage.setItem("refresh_token", "refresh-1");
  });
}
