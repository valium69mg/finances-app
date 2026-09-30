import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createIncomeApi } from "./income";

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
  const income = createIncomeApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
  };
  return { income, call };
}

describe("income api", () => {
  it("lists a month with a limit and the bearer token", async () => {
    const { income, call } = setup(() => json(200, []));
    await expect(income.list("2026-10")).resolves.toEqual([]);
    const { url, init } = call();
    expect(url).toBe("http://x/income?month=2026-10&limit=200");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("creates with decimal strings untouched and returns summary and split", async () => {
    const payload = {
      income: { id: 7, amount: "3383.33", exchange_rate: "17.74", amount_mxn: "60020.27" },
      summary: {
        month: "2026-10",
        month_total_mxn: "60020.27",
        resico: { rate: "0.015", estimated_isr: "900.30405", rate_increased: true, previous_rate: "0.01" },
      },
      split: null,
    };
    const { income, call } = setup(() => json(201, payload));
    const res = await income.create({ amount: "3383.33", currency: "USD", exchange_rate: "17.74" });
    expect(res).toEqual(payload);
    const { url, init, body } = call();
    expect(url).toBe("http://x/income");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ amount: "3383.33", currency: "USD", exchange_rate: "17.74" });
    expect(typeof body.amount).toBe("string");
  });

  it("updates through PUT on the income id", async () => {
    const { income, call } = setup(() => json(200, { income: { id: 3 }, summary: {}, split: null }));
    await income.update(3, { amount: "10" });
    expect(call().url).toBe("http://x/income/3");
    expect(call().init.method).toBe("PUT");
  });

  it("deletes and tolerates the empty 204 body", async () => {
    const { income, call } = setup(() => json(204));
    await expect(income.remove(9)).resolves.toBeUndefined();
    expect(call().url).toBe("http://x/income/9");
    expect(call().init.method).toBe("DELETE");
  });

  it("infers a category and encodes the description", async () => {
    const { income, call } = setup(() => json(200, { category: "Sueldo" }));
    await expect(income.inferCategory("pago & más")).resolves.toBe("Sueldo");
    expect(call().url).toBe("http://x/income/infer-category?description=pago%20%26%20m%C3%A1s");
  });

  it("returns null when no category matches", async () => {
    const { income } = setup(() => json(200, { category: null }));
    await expect(income.inferCategory("zzz")).resolves.toBeNull();
  });

  it("surfaces the backend message on invalid_income", async () => {
    const { income } = setup(() => json(400, { error: "invalid_income", message: "amount must be positive" }));
    const err = await income.create({ amount: "-1" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 400, code: "invalid_income", message: "amount must be positive" });
  });
});
