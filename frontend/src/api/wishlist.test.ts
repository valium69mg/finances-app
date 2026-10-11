import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createWishlistApi, wishlistKeys } from "./wishlist";
import { personalKeys } from "./personal";

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

function setup(handler: (url: string, init: RequestInit) => Response) {
  const fetchFn = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init ?? {}));
  const client = createApiClient({
    baseUrl: "http://x",
    fetchFn: fetchFn as unknown as typeof fetch,
    store: { getAccess: () => "tok", getRefresh: () => null, set: () => {}, clear: () => {} },
  });
  const wishlist = createWishlistApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, body: init.body ? JSON.parse(String(init.body)) : undefined };
  };
  return { wishlist, call };
}

describe("wishlist api", () => {
  it("lists with the bearer token", async () => {
    const { wishlist, call } = setup(() => json(200, []));
    await expect(wishlist.list()).resolves.toEqual([]);
    expect(call().url).toBe("http://x/personal/wishlist");
    expect((call().init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("lists a household user's items for the owner's read-only view", async () => {
    const { wishlist, call } = setup(() => json(200, []));
    await wishlist.list("u1");
    expect(call().url).toBe("http://x/personal/wishlist?user_id=u1");
    expect(wishlistKeys.of("u1")).toEqual([...wishlistKeys.all, "u1"]);
  });

  it("creates and updates through the item body", async () => {
    const { wishlist, call } = setup(() => json(200, { id: 1 }));
    const input = { name: "Bolsa", price: "499.90", category_id: 5, priority: "alta" as const, store_url: "https://tienda.example/b" };
    await wishlist.create(input);
    await wishlist.update(1, input);
    expect(call(0).url).toBe("http://x/personal/wishlist");
    expect(call(0).init.method).toBe("POST");
    expect(call(0).body).toEqual(input);
    expect(call(1).url).toBe("http://x/personal/wishlist/1");
    expect(call(1).init.method).toBe("PUT");
  });

  it("deletes with a 204", async () => {
    const { wishlist, call } = setup(() => json(204));
    await wishlist.remove(3);
    expect(call().url).toBe("http://x/personal/wishlist/3");
    expect(call().init.method).toBe("DELETE");
  });

  it("posts each conversion to its own route", async () => {
    const { wishlist, call } = setup(() => json(200, { id: 1 }));
    await wishlist.convertToFuture(1, { target_amount: "450", due_date: "2026-12-20" });
    await wishlist.convertToExpense(1, { amount: "400", date: "2026-10-09", category_id: 7 });
    await wishlist.convertToRequest(1, { category: "Personal" });
    expect(call(0).url).toBe("http://x/personal/wishlist/1/convert/future-expense");
    expect(call(0).body).toEqual({ target_amount: "450", due_date: "2026-12-20" });
    expect(call(1).url).toBe("http://x/personal/wishlist/1/convert/expense");
    expect(call(1).body).toEqual({ amount: "400", date: "2026-10-09", category_id: 7 });
    expect(call(2).url).toBe("http://x/personal/wishlist/1/convert/request");
    expect(call(2).body).toEqual({ category: "Personal" });
    for (const i of [0, 1, 2]) expect(call(i).init.method).toBe("POST");
  });

  it("surfaces the error code of a conflict", async () => {
    const { wishlist } = setup(() => json(409, { error: "already_converted" }));
    await expect(wishlist.convertToExpense(1, {})).rejects.toSatisfy((e) => e instanceof ApiError && e.status === 409 && e.code === "already_converted");
  });

  it("lives under the personal query key, so any personal change refreshes it", () => {
    expect(wishlistKeys.all.slice(0, personalKeys.all.length)).toEqual([...personalKeys.all]);
  });
});
