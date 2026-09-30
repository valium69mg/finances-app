import { describe, expect, it, vi } from "vitest";
import { createApiClient } from "./client";
import { createFutureExpensesApi } from "./futureExpenses";

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
  const future = createFutureExpensesApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined, headers: (init.headers ?? {}) as Record<string, string> };
  };
  return { future, call };
}

describe("future expenses api", () => {
  it("lists the items with the bearer token", async () => {
    const { future, call } = setup(() => json(200, { active: [], paid: [], totals: {}, free_balance: "0" }));
    await expect(future.list()).resolves.toMatchObject({ free_balance: "0" });
    expect(call().url).toBe("http://x/future-expenses");
    expect(call().init.method).toBe("GET");
    expect(call().headers.Authorization).toBe("Bearer tok");
  });

  it("creates and updates with decimal strings untouched", async () => {
    const { future, call } = setup(() => json(200, { id: 1 }));
    const input = { name: "Laptop", target_amount: "8000.50", due_date: "2027-01-20" };
    await future.create(input);
    await future.update(4, input);
    expect(call(0).url).toBe("http://x/future-expenses");
    expect(call(0).init.method).toBe("POST");
    expect(call(0).body).toEqual(input);
    expect(call(1).url).toBe("http://x/future-expenses/4");
    expect(call(1).init.method).toBe("PUT");
  });

  it("deletes, contributes, assigns and pays on the item routes", async () => {
    const { future, call } = setup((_url, init) => (init.method === "DELETE" ? json(204) : json(200, {})));
    await future.remove(4);
    await future.addSaving(4, { amount: "100", date: "2026-10-01" });
    await future.assign(4, { amount: "25.50" });
    await future.pay(4, { amount: "7900", category: "Ocio" });
    expect(call(0)).toMatchObject({ url: "http://x/future-expenses/4", init: { method: "DELETE" } });
    expect(call(1)).toMatchObject({ url: "http://x/future-expenses/4/savings", body: { amount: "100", date: "2026-10-01" } });
    expect(call(2)).toMatchObject({ url: "http://x/future-expenses/4/assign", body: { amount: "25.50" } });
    expect(call(3)).toMatchObject({ url: "http://x/future-expenses/4/pay", body: { amount: "7900", category: "Ocio" } });
  });
});
