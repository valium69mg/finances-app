import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createSavingsApi } from "./savings";

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
  const savings = createSavingsApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
  };
  return { savings, call };
}

describe("savings api", () => {
  it("lists a month with a limit and the bearer token", async () => {
    const { savings, call } = setup(() => json(200, []));
    await expect(savings.list("2026-10")).resolves.toEqual([]);
    const { url, init } = call();
    expect(url).toBe("http://x/savings?month=2026-10&limit=200");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("creates keeping a negative decimal string untouched", async () => {
    const { savings, call } = setup(() => json(201, { id: 1, amount: "-250.50" }));
    await savings.create({ amount: "-250.50", instrument: "voo" });
    const { url, init, body } = call();
    expect(url).toBe("http://x/savings");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ amount: "-250.50", instrument: "voo" });
    expect(typeof body.amount).toBe("string");
  });

  it("updates through PUT and deletes tolerating the empty 204 body", async () => {
    const { savings, call } = setup((url) => (url.endsWith("/9") ? json(204) : json(200, { id: 3 })));
    await savings.update(3, { amount: "10" });
    expect(call(0).url).toBe("http://x/savings/3");
    expect(call(0).init.method).toBe("PUT");
    await expect(savings.remove(9)).resolves.toBeUndefined();
    expect(call(1).url).toBe("http://x/savings/9");
    expect(call(1).init.method).toBe("DELETE");
  });

  it("transfers and returns both legs", async () => {
    const payload = { out: { id: 1, amount: "-100" }, in: { id: 2, amount: "100" } };
    const { savings, call } = setup(() => json(201, payload));
    await expect(savings.transfer({ from: "voo", to: "cetes", amount: "100" })).resolves.toEqual(payload);
    const { url, init, body } = call();
    expect(url).toBe("http://x/savings/transfers");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ from: "voo", to: "cetes", amount: "100" });
  });

  it("appends and lists valuations", async () => {
    const { savings, call } = setup((url) => (url.endsWith("/valuations") ? json(200, []) : json(200, [])));
    await savings.addValuation({ instrument: "voo", value_mxn: "1234.50", date: "2026-10-01", note: "GBM" });
    expect(call(0).url).toBe("http://x/savings/valuations");
    expect(call(0).init.method).toBe("POST");
    expect(call(0).body).toEqual({ instrument: "voo", value_mxn: "1234.50", date: "2026-10-01", note: "GBM" });
    await savings.listValuations();
    expect(call(1).url).toBe("http://x/savings/valuations");
    expect(call(1).init.method ?? "GET").toBe("GET");
  });

  it("reads the portfolio", async () => {
    const payload = { rows: [], total_contributed: "0", total_value: "0", by_type: [], by_destination: [], emergency: { accumulated: "0", goal: "0" } };
    const { savings, call } = setup(() => json(200, payload));
    await expect(savings.portfolio()).resolves.toEqual(payload);
    expect(call().url).toBe("http://x/savings/portfolio");
  });

  it("surfaces the backend message on invalid_saving", async () => {
    const { savings } = setup(() => json(400, { error: "invalid_saving", message: "amount must not be zero" }));
    const err = await savings.create({ amount: "0" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 400, code: "invalid_saving", message: "amount must not be zero" });
  });
});
