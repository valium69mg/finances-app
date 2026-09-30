import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createMonthCloseApi } from "./monthClose";

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
  const monthClose = createMonthCloseApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, headers: (init.headers ?? {}) as Record<string, string> };
  };
  return { monthClose, call };
}

describe("month close api", () => {
  it("previews the given period, or lets the backend default to the previous month", async () => {
    const { monthClose, call } = setup(() => json(200, { preview: { period: "2026-09" }, existing: null }));
    await expect(monthClose.preview("2026-09")).resolves.toEqual({ preview: { period: "2026-09" }, existing: null });
    await monthClose.preview();
    expect(call(0).url).toBe("http://x/month-close/preview?period=2026-09");
    expect(call(0).init.method).toBe("GET");
    expect(call(0).headers.Authorization).toBe("Bearer tok");
    expect(call(1).url).toBe("http://x/month-close/preview");
  });

  it("creates a close with the period as the JSON body", async () => {
    const { monthClose, call } = setup(() => json(201, { period: "2026-09" }));
    await expect(monthClose.create("2026-09")).resolves.toEqual({ period: "2026-09" });
    expect(call().url).toBe("http://x/month-close");
    expect(call().init.method).toBe("POST");
    expect(JSON.parse(String(call().init.body))).toEqual({ period: "2026-09" });
  });

  it("lists and gets stored closes", async () => {
    const { monthClose, call } = setup(() => json(200, []));
    await expect(monthClose.list()).resolves.toEqual([]);
    await monthClose.get("2026-09");
    expect(call(0).url).toBe("http://x/month-close");
    expect(call(1).url).toBe("http://x/month-close/2026-09");
    expect(call(1).init.method).toBe("GET");
  });

  it("deletes a close and resolves on 204", async () => {
    const { monthClose, call } = setup(() => json(204));
    await expect(monthClose.remove("2026-09")).resolves.toBeUndefined();
    expect(call().url).toBe("http://x/month-close/2026-09");
    expect(call().init.method).toBe("DELETE");
  });

  it("surfaces the error code of a rejected close", async () => {
    const { monthClose } = setup(() => json(409, { error: "already_closed", message: "month already closed: 2026-09" }));
    const err = await monthClose.create("2026-09").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(409);
    expect((err as ApiError).code).toBe("already_closed");
  });
});
