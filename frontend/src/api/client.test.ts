import { describe, expect, it, vi } from "vitest";
import { createApiClient, ApiError } from "./client";

function memoryStore(access: string | null, refresh: string | null) {
  const s = { access, refresh };
  return {
    s,
    getAccess: () => s.access,
    getRefresh: () => s.refresh,
    set: (p: { access_token: string; refresh_token: string }) => {
      s.access = p.access_token;
      s.refresh = p.refresh_token;
    },
    clear: () => {
      s.access = null;
      s.refresh = null;
    },
  };
}

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

function setup(handler: (url: string, init: RequestInit) => Response) {
  const store = memoryStore("old", "r1");
  const fetchFn = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init ?? {}));
  const onSessionExpired = vi.fn();
  const client = createApiClient({ baseUrl: "http://api", fetchFn: fetchFn as unknown as typeof fetch, store, onSessionExpired });
  return { store, fetchFn, onSessionExpired, client };
}

describe("api client", () => {
  it("attaches the bearer token", async () => {
    const { client, fetchFn } = setup(() => json(200, { ok: true }));
    await client.request("/auth/me");
    const init = fetchFn.mock.calls[0][1] as RequestInit;
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer old");
  });

  it("refreshes once on 401 and retries with the new token", async () => {
    const { client, fetchFn, store } = setup((url, init) => {
      if (url.endsWith("/auth/refresh")) return json(200, { access_token: "new", refresh_token: "r2" });
      const auth = (init.headers as Record<string, string>).Authorization;
      return auth === "Bearer new" ? json(200, { ok: 1 }) : json(401, { error: "unauthorized" });
    });
    await expect(client.request("/auth/me")).resolves.toEqual({ ok: 1 });
    expect(store.s).toEqual({ access: "new", refresh: "r2" });
    expect(fetchFn).toHaveBeenCalledTimes(3);
  });

  it("shares one refresh call between concurrent 401s (single-flight)", async () => {
    const { client, fetchFn } = setup((url, init) => {
      if (url.endsWith("/auth/refresh")) return json(200, { access_token: "new", refresh_token: "r2" });
      const auth = (init.headers as Record<string, string>).Authorization;
      return auth === "Bearer new" ? json(200, {}) : json(401, { error: "unauthorized" });
    });
    await Promise.all([client.request("/a"), client.request("/b"), client.request("/c")]);
    const refreshCalls = fetchFn.mock.calls.filter((c) => String(c[0]).endsWith("/auth/refresh"));
    expect(refreshCalls).toHaveLength(1);
  });

  it("clears tokens and signals expiry when refresh fails", async () => {
    const { client, store, onSessionExpired } = setup((url) =>
      url.endsWith("/auth/refresh") ? json(401, { error: "invalid_token" }) : json(401, { error: "unauthorized" }),
    );
    await expect(client.request("/auth/me")).rejects.toBeInstanceOf(ApiError);
    expect(store.s).toEqual({ access: null, refresh: null });
    expect(onSessionExpired).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["rate limited", () => json(429, { error: "rate_limited" })],
    ["server error", () => json(503, { error: "internal_error" })],
  ])("keeps the session when the refresh endpoint is %s", async (_name, refreshResponse) => {
    const { client, store, onSessionExpired } = setup((url) =>
      url.endsWith("/auth/refresh") ? refreshResponse() : json(401, { error: "unauthorized" }),
    );
    await expect(client.request("/auth/me")).rejects.toMatchObject({ status: 401 });
    expect(store.s).toEqual({ access: "old", refresh: "r1" });
    expect(onSessionExpired).not.toHaveBeenCalled();
  });

  it("keeps the session when the refresh call fails on the network", async () => {
    const store = memoryStore("old", "r1");
    const fetchFn = vi.fn(async (url: RequestInfo | URL) => {
      if (String(url).endsWith("/auth/refresh")) throw new TypeError("network down");
      return json(401, { error: "unauthorized" });
    });
    const onSessionExpired = vi.fn();
    const client = createApiClient({ baseUrl: "http://api", fetchFn: fetchFn as unknown as typeof fetch, store, onSessionExpired });
    await expect(client.request("/auth/me")).rejects.toBeInstanceOf(ApiError);
    expect(store.s).toEqual({ access: "old", refresh: "r1" });
    expect(onSessionExpired).not.toHaveBeenCalled();
  });

  it("does not retry more than once when the retry also returns 401", async () => {
    const { client, fetchFn, onSessionExpired } = setup((url) =>
      url.endsWith("/auth/refresh") ? json(200, { access_token: "new", refresh_token: "r2" }) : json(401, { error: "unauthorized" }),
    );
    await expect(client.request("/auth/me")).rejects.toBeInstanceOf(ApiError);
    expect(fetchFn).toHaveBeenCalledTimes(3);
    expect(onSessionExpired).toHaveBeenCalledTimes(1);
  });

  it("does not refresh for anonymous requests", async () => {
    const { client, fetchFn } = setup(() => json(401, { error: "invalid_credentials" }));
    await expect(client.request("/auth/login", { body: {}, anonymous: true })).rejects.toMatchObject({ status: 401, code: "invalid_credentials" });
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });
});
