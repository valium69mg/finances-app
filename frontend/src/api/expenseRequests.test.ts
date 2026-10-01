import { describe, expect, it, vi } from "vitest";
import { createApiClient } from "./client";
import { createExpenseRequestsApi } from "./expenseRequests";

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function setup(handler: (url: string, init: RequestInit) => Response = () => json(200, {})) {
  const fetchFn = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init ?? {}));
  const client = createApiClient({
    baseUrl: "http://x",
    fetchFn: fetchFn as unknown as typeof fetch,
    store: { getAccess: () => "tok", getRefresh: () => null, set: () => {}, clear: () => {} },
  });
  const requests = createExpenseRequestsApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined, headers: (init.headers ?? {}) as Record<string, string> };
  };
  return { requests, call };
}

describe("expense requests api", () => {
  it("lists with the bearer token and an optional status filter", async () => {
    const { requests, call } = setup(() => json(200, []));
    await requests.list();
    await requests.list("solicitada");
    expect(call(0).url).toBe("http://x/expense-requests");
    expect(call(0).headers.Authorization).toBe("Bearer tok");
    expect(call(1).url).toBe("http://x/expense-requests?status=solicitada");
  });

  it("creates with the amount as the typed decimal string", async () => {
    const { requests, call } = setup();
    await requests.create({ amount: "250.50", description: "Tacos", suggested_category: "Comida", date: "2026-10-03" });
    expect(call().url).toBe("http://x/expense-requests");
    expect(call().init.method).toBe("POST");
    expect(call().body).toEqual({ amount: "250.50", description: "Tacos", suggested_category: "Comida", date: "2026-10-03" });
  });

  it("reads the Gasto categories and cancels by id", async () => {
    const { requests, call } = setup(() => json(200, ["Ocio"]));
    await expect(requests.categories()).resolves.toEqual(["Ocio"]);
    await requests.cancel(7);
    expect(call(0).url).toBe("http://x/expense-requests/categories");
    expect(call(1).url).toBe("http://x/expense-requests/7/cancel");
    expect(call(1).init.method).toBe("POST");
  });

  it("asks for the budget check with the encoded category and the date", async () => {
    const { requests, call } = setup();
    await requests.budgetCheck(7, "Comida fuera", "2026-10-20");
    await requests.budgetCheck(7, "Ocio", "");
    expect(call(0).url).toBe("http://x/expense-requests/7/budget-check?category=Comida+fuera&date=2026-10-20");
    expect(call(1).url).toBe("http://x/expense-requests/7/budget-check?category=Ocio");
  });

  it("approves each destination and rejects with the comment", async () => {
    const { requests, call } = setup();
    await requests.approve(7, { destination: "gasto", category: "Ocio", date: "2026-10-20", payment_method: "Efectivo" });
    await requests.approve(7, { destination: "gasto_futuro", due_date: "2026-12-20" });
    await requests.reject(7, "No este mes");
    expect(call(0).url).toBe("http://x/expense-requests/7/approve");
    expect(call(0).body).toEqual({ destination: "gasto", category: "Ocio", date: "2026-10-20", payment_method: "Efectivo" });
    expect(call(1).body).toEqual({ destination: "gasto_futuro", due_date: "2026-12-20" });
    expect(call(2).url).toBe("http://x/expense-requests/7/reject");
    expect(call(2).body).toEqual({ comment: "No este mes" });
  });
});
