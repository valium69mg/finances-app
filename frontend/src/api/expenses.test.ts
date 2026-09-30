import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createExpensesApi } from "./expenses";

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
  const expenses = createExpensesApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
  };
  return { expenses, call };
}

describe("expenses api", () => {
  it("lists a month with a limit and the bearer token", async () => {
    const { expenses, call } = setup(() => json(200, []));
    await expect(expenses.list("2026-10")).resolves.toEqual([]);
    const { url, init } = call();
    expect(url).toBe("http://x/expenses?month=2026-10&limit=200");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("creates with decimal strings untouched", async () => {
    const payload = {
      expense: { id: 7, amount: "62.03", exchange_rate: "17.74", amount_mxn: "1100.4122" },
      budget: null,
    };
    const { expenses, call } = setup(() => json(201, payload));
    const res = await expenses.create({ amount: "62.03", currency: "USD", exchange_rate: "17.74" });
    expect(res).toEqual(payload);
    const { url, init, body } = call();
    expect(url).toBe("http://x/expenses");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ amount: "62.03", currency: "USD", exchange_rate: "17.74" });
    expect(typeof body.amount).toBe("string");
  });

  it("updates through PUT on the expense id", async () => {
    const { expenses, call } = setup(() => json(200, { expense: { id: 3 }, budget: null }));
    await expenses.update(3, { amount: "10" });
    expect(call().url).toBe("http://x/expenses/3");
    expect(call().init.method).toBe("PUT");
  });

  it("deletes and tolerates the empty 204 body", async () => {
    const { expenses, call } = setup(() => json(204));
    await expect(expenses.remove(9)).resolves.toBeUndefined();
    expect(call().url).toBe("http://x/expenses/9");
    expect(call().init.method).toBe("DELETE");
    expect(call().init.body).toBeUndefined();
  });

  it("infers a category and encodes the description", async () => {
    const { expenses, call } = setup(() => json(200, { category: "Mandado" }));
    await expect(expenses.inferCategory("super & más")).resolves.toBe("Mandado");
    expect(call().url).toBe("http://x/expenses/infer-category?description=super%20%26%20m%C3%A1s");
  });

  it("returns null when no category matches", async () => {
    const { expenses } = setup(() => json(200, { category: null }));
    await expect(expenses.inferCategory("zzz")).resolves.toBeNull();
  });

  it("surfaces the backend message on invalid_expense", async () => {
    const { expenses } = setup(() => json(400, { error: "invalid_expense", message: "amount must be positive" }));
    const err = await expenses.create({ amount: "-1" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 400, code: "invalid_expense", message: "amount must be positive" });
  });
});
