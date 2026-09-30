import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "./client";
import {
  deletePause,
  getMonthBudgets,
  getSettings,
  updateBrackets,
  updateCategories,
  updateGeneral,
  updatePause,
  type Category,
  type General,
} from "./settings";

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function mockFetch(handler: (url: string, init: RequestInit) => Response | Promise<Response>) {
  const fn = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init ?? {}));
  vi.stubGlobal("fetch", fn);
  return fn;
}

function call(fn: ReturnType<typeof mockFetch>, index = 0) {
  const [url, init] = fn.mock.calls[index] as [string, RequestInit];
  return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
}

describe("settings api", () => {
  beforeEach(() => localStorage.setItem("access_token", "tok"));
  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("getSettings sends the bearer token and returns the body untouched", async () => {
    const payload = { general: { salary_usd: "1234.50" }, categories: [] };
    const fn = mockFetch(() => json(200, payload));
    await expect(getSettings()).resolves.toEqual(payload);
    const { url, init } = call(fn);
    expect(url).toMatch(/\/settings$/);
    expect(init.method).toBe("GET");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("keeps money as decimal strings on the wire", async () => {
    const fn = mockFetch(() => json(204));
    const cats: Category[] = [{ name: "Renta", kind: "fixed", budget: "12345.67", includes: "", keywords: ["a"] }];
    await updateCategories(cats);
    const { url, init, body } = call(fn);
    expect(url).toMatch(/\/settings\/categories$/);
    expect(init.method).toBe("PUT");
    expect(body[0].budget).toBe("12345.67");
    expect(typeof body[0].budget).toBe("string");
  });

  it("sends a null budget as null", async () => {
    const fn = mockFetch(() => json(204));
    await updateCategories([{ name: "X", kind: "variable", budget: null, includes: "", keywords: [] }]);
    expect(call(fn).body[0].budget).toBeNull();
  });

  it("puts each section on its own route", async () => {
    const fn = mockFetch(() => json(204));
    const general: General = {
      salary_usd: "1000",
      fx_rate_applied: "17.5",
      morse_fee_rate: "0.01",
      emergency_months: "6",
      extra_income_estimate_mxn: "0",
      budget_includes_extra_income: false,
      extra_income_split: { ahorro: "0.5" },
      investment_allocation: [{ key: "VOO", value: "1" }],
    };
    await updateGeneral(general);
    await updateBrackets([{ upper: "25000.00", rate: "0.0125" }]);
    await updatePause({
      months: ["2026-10"],
      normal_budget: "5000",
      resume_month: "2027-02",
      future_expenses_plan: { "2026-10": "12000" },
      note: "",
    });
    expect(call(fn, 0).url).toMatch(/\/settings\/general$/);
    expect(call(fn, 0).body).toEqual(general);
    expect(call(fn, 1).url).toMatch(/\/settings\/brackets$/);
    expect(call(fn, 1).body).toEqual([{ upper: "25000.00", rate: "0.0125" }]);
    expect(call(fn, 2).url).toMatch(/\/settings\/investment-pause$/);
    expect(call(fn, 2).init.method).toBe("PUT");
  });

  it("deletePause issues DELETE without a body", async () => {
    const fn = mockFetch(() => json(204));
    await expect(deletePause()).resolves.toBeUndefined();
    const { url, init } = call(fn);
    expect(url).toMatch(/\/settings\/investment-pause$/);
    expect(init.method).toBe("DELETE");
    expect(init.body).toBeUndefined();
  });

  it("getMonthBudgets encodes the month query", async () => {
    const fn = mockFetch(() => json(200, [{ name: "Inversiones", kind: "savings", budget: "0" }]));
    await expect(getMonthBudgets("2026-10")).resolves.toHaveLength(1);
    expect(call(fn).url).toMatch(/\/settings\/budgets\?month=2026-10$/);
  });

  it("surfaces the backend message on invalid_settings", async () => {
    mockFetch(() => json(400, { error: "invalid_settings", message: "budget must not be negative" }));
    await expect(updateCategories([])).rejects.toMatchObject({
      name: "ApiError",
      status: 400,
      code: "invalid_settings",
      message: "budget must not be negative",
    });
  });

  it("falls back to the code as message when the body has none", async () => {
    mockFetch(() => json(409, { error: "settings_incomplete" }));
    await expect(getSettings()).rejects.toMatchObject({ status: 409, code: "settings_incomplete", message: "settings_incomplete" });
  });

  it("accepts an injected fetch on the api client", async () => {
    const fetchFn = vi.fn(async () => json(204));
    const client = createApiClient({
      baseUrl: "http://x",
      fetchFn: fetchFn as unknown as typeof fetch,
      store: { getAccess: () => null, getRefresh: () => null, set: () => {}, clear: () => {} },
    });
    await client.request("/settings/investment-pause", { method: "DELETE" });
    expect(fetchFn).toHaveBeenCalledWith("http://x/settings/investment-pause", expect.objectContaining({ method: "DELETE" }));
  });
});
