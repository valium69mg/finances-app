import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createDashboardApi } from "./dashboard";

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function setup(handler: (url: string, init: RequestInit) => Response) {
  const fetchFn = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init ?? {}));
  const client = createApiClient({
    baseUrl: "http://x",
    fetchFn: fetchFn as unknown as typeof fetch,
    store: { getAccess: () => "tok", getRefresh: () => null, set: () => {}, clear: () => {} },
  });
  const dashboard = createDashboardApi(client);
  const call = (i = 0) => fetchFn.mock.calls[i] as [string, RequestInit];
  return { dashboard, call };
}

describe("dashboard api", () => {
  it("gets a month with the bearer token and returns decimal strings untouched", async () => {
    const payload = {
      month: "2026-10",
      period_start: "2026-09-30",
      period_end: "2026-10-30",
      categories: [{ category: "Mandado", spent: "1500.50", budget: "1000", remaining: "-500.50", over_budget: true }],
      income: "50000",
      expenses: "1500.50",
      savings: "5000",
      available: "43499.50",
      emergency: { accumulated: "10000", goal: "120000" },
      tax: { rate: "0.011", estimated_isr: "550", filing_status: "pendiente", previous_period: "2026-09", previous_period_pending: true },
      cycle: { today: "2026-10-15", day: 16, days: 31 },
      future_expenses: {
        items: [{ name: "Predial", due_date: "2027-01-20", target: "10000", saved: "2500.50", remaining: "7499.50", suggested_monthly: "1874.88", cycles_left: 4 }],
        target: "10000",
        saved: "2500.50",
        remaining: "7499.50",
        suggested_monthly: "1874.88",
      },
      upcoming_bills: [{ id: 1, name: "Luz", category: "Servicios", amount: null, currency: "MXN", due_date: "2026-10-17", days_until_due: 2, overdue: false }],
      recent_movements: [{ id: 9, date: "2026-10-14", kind: "Ahorro", description: "Apartado", category: "Gastos futuros", amount_mxn: "2500.50" }],
    };
    const { dashboard, call } = setup(() => json(200, payload));
    await expect(dashboard.get("2026-10")).resolves.toEqual(payload);
    const [url, init] = call();
    expect(url).toBe("http://x/dashboard?month=2026-10");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("keeps a null tax when the tax settings are incomplete", async () => {
    const { dashboard } = setup(() => json(200, { month: "2026-10", categories: [], tax: null }));
    await expect(dashboard.get("2026-10")).resolves.toMatchObject({ tax: null });
  });

  it("surfaces settings_incomplete with its status", async () => {
    const { dashboard } = setup(() => json(422, { error: "settings_incomplete", message: "missing required config: emergency_months" }));
    const err = await dashboard.get("2026-10").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 422, code: "settings_incomplete" });
  });
});
