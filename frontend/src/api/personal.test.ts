import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createPersonalApi } from "./personal";

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
  const personal = createPersonalApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
  };
  return { personal, call };
}

describe("personal api", () => {
  it("lists the categories of a kind with the bearer token", async () => {
    const cats = [{ id: 1, kind: "Gasto", name: "Ropa", fixed: false }];
    const { personal, call } = setup(() => json(200, cats));
    await expect(personal.categories("Gasto")).resolves.toEqual(cats);
    expect(call().url).toBe("http://x/personal/categories?kind=Gasto");
    expect((call().init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("creates and renames a category through the name body", async () => {
    const { personal, call } = setup(() => json(200, { id: 9, kind: "Gasto", name: "Café", fixed: false }));
    await personal.createCategory("Café");
    await personal.renameCategory(9, "Cafetería");
    expect(call(0).url).toBe("http://x/personal/categories");
    expect(call(0).init.method).toBe("POST");
    expect(call(0).body).toEqual({ name: "Café" });
    expect(call(1).url).toBe("http://x/personal/categories/9");
    expect(call(1).init.method).toBe("PUT");
    expect(call(1).body).toEqual({ name: "Cafetería" });
  });

  it("lists movements with month, kind and category filters", async () => {
    const { personal, call } = setup(() => json(200, []));
    await personal.movements({ month: "2026-10", kind: "Gasto", categoryId: 4 });
    expect(call().url).toBe("http://x/personal/movements?month=2026-10&kind=Gasto&category_id=4");
  });

  it("omits an empty month so the server picks the current cycle", async () => {
    const { personal, call } = setup(() => json(200, []));
    await personal.movements({ month: "", kind: "Ingreso" });
    await personal.movements();
    expect(call(0).url).toBe("http://x/personal/movements?kind=Ingreso");
    expect(call(1).url).toBe("http://x/personal/movements");
  });

  it("creates keeping the amount a string, including a negative withdrawal", async () => {
    const { personal, call } = setup(() => json(201, { id: 1 }));
    await personal.createMovement({ kind: "Ahorro", category_id: 2, amount: "-500.50", description: "Retiro", date: "2026-10-05" });
    const { url, init, body } = call();
    expect(url).toBe("http://x/personal/movements");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ kind: "Ahorro", category_id: 2, amount: "-500.50", description: "Retiro", date: "2026-10-05" });
    expect(typeof body.amount).toBe("string");
  });

  it("updates through PUT and deletes tolerating the empty 204 body", async () => {
    const { personal, call } = setup((_, init) => (init.method === "DELETE" ? json(204) : json(200, { id: 3 })));
    await personal.updateMovement(3, { kind: "Gasto", category_id: 1, amount: "10" });
    await expect(personal.removeMovement(3)).resolves.toBeUndefined();
    expect(call(0).url).toBe("http://x/personal/movements/3");
    expect(call(0).init.method).toBe("PUT");
    expect(call(1).init.method).toBe("DELETE");
    expect(call(1).init.body).toBeUndefined();
  });

  it("surfaces the backend error code", async () => {
    const { personal } = setup(() => json(409, { error: "category_exists", message: "category name already exists" }));
    const err = await personal.createCategory("Ropa").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: "category_exists" });
  });
  it("talks to the future expenses routes under /personal", async () => {
    const { personal, call } = setup(() => json(200, {}));
    const input = { name: "Viaje", target_amount: "1000", due_date: "2026-12-20" };
    await personal.futureList();
    await personal.futureCreate(input);
    await personal.futureUpdate(3, input);
    await personal.futureRemove(3);
    await personal.futureAddSaving(3, { amount: "100", date: "2026-10-10" });
    await personal.futureAssign(3, { amount: "50" });
    await personal.futurePay(3, { amount: "300", date: "2026-10-05", category_id: 5 });
    const seen = [0, 1, 2, 3, 4, 5, 6].map((i) => `${call(i).init.method ?? "GET"} ${call(i).url.replace("http://x", "")}`);
    expect(seen).toEqual([
      "GET /personal/future-expenses",
      "POST /personal/future-expenses",
      "PUT /personal/future-expenses/3",
      "DELETE /personal/future-expenses/3",
      "POST /personal/future-expenses/3/savings",
      "POST /personal/future-expenses/3/assign",
      "POST /personal/future-expenses/3/pay",
    ]);
    expect(call(1).body).toEqual(input);
    expect(call(6).body).toEqual({ amount: "300", date: "2026-10-05", category_id: 5 });
  });
});
