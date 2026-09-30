import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createBillsApi, type BillInput } from "./bills";

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
  const bills = createBillsApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, headers: (init.headers ?? {}) as Record<string, string> };
  };
  return { bills, call };
}

const INPUT: BillInput = {
  name: "Megacable",
  category: "Servicios",
  amount: "550.00",
  currency: "MXN",
  recurrence: "monthly",
  next_due_date: "2026-11-01",
  reminder_lead_days: 3,
  active: true,
  notes: "",
};

describe("bills api", () => {
  it("lists active bills by default and inactive ones on request", async () => {
    const { bills, call } = setup(() => json(200, []));
    await expect(bills.list()).resolves.toEqual([]);
    await bills.list(true);
    expect(call(0).url).toBe("http://x/bills");
    expect(call(0).init.method).toBe("GET");
    expect(call(0).headers.Authorization).toBe("Bearer tok");
    expect(call(1).url).toBe("http://x/bills?include_inactive=true");
  });

  it("gets a bill with its history", async () => {
    const { bills, call } = setup(() => json(200, { id: 7, history: [] }));
    await bills.get(7);
    expect(call().url).toBe("http://x/bills/7");
  });

  it("creates with decimal strings and a null amount untouched", async () => {
    const { bills, call } = setup(() => json(201, { id: 1 }));
    await bills.create(INPUT);
    await bills.create({ ...INPUT, amount: null });
    expect(call(0).url).toBe("http://x/bills");
    expect(call(0).init.method).toBe("POST");
    expect(call(0).headers["Content-Type"]).toBe("application/json");
    expect(JSON.parse(String(call(0).init.body))).toEqual(INPUT);
    expect(JSON.parse(String(call(1).init.body)).amount).toBeNull();
  });

  it("updates with PUT", async () => {
    const { bills, call } = setup(() => json(200, { id: 7 }));
    await bills.update(7, INPUT);
    expect(call().url).toBe("http://x/bills/7");
    expect(call().init.method).toBe("PUT");
  });

  it("deactivates with DELETE and accepts the empty response", async () => {
    const { bills, call } = setup(() => json(204));
    await expect(bills.deactivate(7)).resolves.toBeUndefined();
    expect(call().url).toBe("http://x/bills/7");
    expect(call().init.method).toBe("DELETE");
  });

  it("pays sending only the fields given", async () => {
    const { bills, call } = setup(() => json(200, {}));
    await bills.pay(7, {});
    await bills.pay(7, { date: "2026-10-30", amount: "499.50", category: "Hogar", description: "descuento" });
    expect(call(0).url).toBe("http://x/bills/7/pay");
    expect(call(0).init.method).toBe("POST");
    expect(JSON.parse(String(call(0).init.body))).toEqual({});
    expect(JSON.parse(String(call(1).init.body))).toEqual({ date: "2026-10-30", amount: "499.50", category: "Hogar", description: "descuento" });
  });

  it("skips with a POST and no body", async () => {
    const { bills, call } = setup(() => json(200, { id: 7 }));
    await bills.skip(7);
    expect(call().url).toBe("http://x/bills/7/skip");
    expect(call().init.method).toBe("POST");
    expect(call().init.body).toBeUndefined();
  });

  it("surfaces the API error code and message", async () => {
    const { bills } = setup(() => json(409, { error: "bill_inactive", message: "the bill is inactive" }));
    await expect(bills.pay(7, {})).rejects.toMatchObject({ status: 409, code: "bill_inactive", message: "the bill is inactive" } satisfies Partial<ApiError>);
  });
});
