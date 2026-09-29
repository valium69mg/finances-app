import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "./client";
import { fetchMe, identify, login, logout } from "./auth";

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

function lastRequest(fn: ReturnType<typeof mockFetch>, index = 0) {
  const [url, init] = fn.mock.calls[index] as [string, RequestInit];
  return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
}

describe("identify", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.unstubAllGlobals());

  it.each(["password_required", "verification_sent"] as const)("returns %s", async (status) => {
    mockFetch(() => json(200, { status }));
    await expect(identify("a@b.co")).resolves.toBe(status);
  });

  it("posts only the email to /auth/identify, anonymously", async () => {
    localStorage.setItem("access_token", "stale");
    const fn = mockFetch(() => json(200, { status: "password_required" }));
    await identify("a@b.co");
    const { url, init, body } = lastRequest(fn);
    expect(url).toMatch(/\/auth\/identify$/);
    expect(init.method).toBe("POST");
    expect(body).toEqual({ email: "a@b.co" });
    expect((init.headers as Record<string, string>).Authorization).toBeUndefined();
  });

  it.each([
    [400, "invalid_email"],
    [429, "rate_limited"],
    [500, "internal_error"],
  ])("rejects with ApiError(%i, %s)", async (status, code) => {
    mockFetch(() => json(status, { error: code }));
    await expect(identify("a@b.co")).rejects.toMatchObject({ name: "ApiError", status, code });
  });

  it("does not try to refresh a session on 401", async () => {
    localStorage.setItem("refresh_token", "r");
    const fn = mockFetch(() => json(401, { error: "x" }));
    await expect(identify("a@b.co")).rejects.toBeInstanceOf(ApiError);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it.each([{ status: "something_else" }, {}, { status: 1 }, null])("rejects an unexpected body %j", async (body) => {
    mockFetch(() => json(200, body));
    await expect(identify("a@b.co")).rejects.toMatchObject({ code: "unexpected_response" });
  });

  it("propagates network failures", async () => {
    mockFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    await expect(identify("a@b.co")).rejects.toBeInstanceOf(TypeError);
  });
});

describe("login", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.unstubAllGlobals());

  it("stores the token pair on success", async () => {
    const fn = mockFetch(() => json(200, { access_token: "a1", refresh_token: "r1" }));
    await expect(login("a@b.co", "pw")).resolves.toBeUndefined();
    expect(lastRequest(fn).body).toEqual({ email: "a@b.co", password: "pw" });
    expect(lastRequest(fn).url).toMatch(/\/auth\/login$/);
    expect(localStorage.getItem("access_token")).toBe("a1");
    expect(localStorage.getItem("refresh_token")).toBe("r1");
  });

  it.each([
    [401, "invalid_credentials"],
    [429, "rate_limited"],
    [400, "invalid_email"],
  ])("rejects with ApiError(%i, %s) and stores nothing", async (status, code) => {
    mockFetch(() => json(status, { error: code }));
    await expect(login("a@b.co", "pw")).rejects.toMatchObject({ status, code });
    expect(localStorage.getItem("access_token")).toBeNull();
  });

  it("does not treat a legacy 202 body as a session", async () => {
    mockFetch(() => json(202, { status: "verification_pending" }));
    await expect(login("a@b.co", "pw")).rejects.toMatchObject({ code: "unexpected_response" });
    expect(localStorage.getItem("access_token")).toBeNull();
  });

  it("rejects a 200 without tokens", async () => {
    mockFetch(() => json(200, { access_token: "a1" }));
    await expect(login("a@b.co", "pw")).rejects.toMatchObject({ code: "unexpected_response" });
    expect(localStorage.getItem("access_token")).toBeNull();
  });
});

describe("logout and fetchMe", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.unstubAllGlobals());

  it("clears the session and revokes the refresh token", async () => {
    localStorage.setItem("access_token", "a");
    localStorage.setItem("refresh_token", "r");
    const fn = mockFetch(() => json(204));
    await logout();
    expect(localStorage.getItem("access_token")).toBeNull();
    expect(lastRequest(fn).body).toEqual({ refresh_token: "r" });
  });

  it("clears the session even if the server call fails", async () => {
    localStorage.setItem("refresh_token", "r");
    mockFetch(() => json(500, { error: "x" }));
    await expect(logout()).resolves.toBeUndefined();
    expect(localStorage.getItem("refresh_token")).toBeNull();
  });

  it("fetchMe sends the bearer token", async () => {
    localStorage.setItem("access_token", "tok");
    const fn = mockFetch(() => json(200, { id: "u1", email: "a@b.co", verified: true }));
    await expect(fetchMe()).resolves.toMatchObject({ id: "u1" });
    expect((lastRequest(fn).init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });
});
