import type { Request, Route } from "@playwright/test";

/**
 * In-memory stand-in for the /users endpoints, used by helpers.ts. It keeps what the backend guarantees: an
 * email is unique, the owner cannot deactivate themselves, a verified or inactive account cannot be invited and
 * the invitation is rate limited. All examples are fake: the repository is public.
 */

export interface MockUser {
  id: string;
  email: string;
  role: "owner" | "household";
  active: boolean;
  verified: boolean;
  created_at?: string;
}

export type UsersFail = "list" | "invite" | "create";

type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

/** The signed-in owner of the mock (GET /auth/me answers with it). */
export const OWNER_USER: MockUser = { id: "u-owner", email: "admin@example.com", role: "owner", active: true, verified: true, created_at: "2026-09-01T00:00:00Z" };

export function createUsersMock(seed: MockUser[], fail: UsersFail | undefined, json: Json) {
  const users: MockUser[] = structuredClone(seed.length > 0 ? seed : [OWNER_USER]).map((u) => ({ created_at: "2026-09-02T00:00:00Z", ...u }));
  const writes: { method: string; path: string; body: unknown }[] = [];
  let next = 1;

  async function handle(route: Route, request: Request, pathname: string): Promise<boolean> {
    if (!pathname.startsWith("/users")) return false;
    const method = request.method();

    if (pathname === "/users" && method === "GET") {
      if (fail === "list") await json(route, 500, { error: "internal_error" });
      else await json(route, 200, users);
      return true;
    }
    if (pathname === "/users" && method === "POST") {
      const body = request.postDataJSON() as { email: string; role: string };
      writes.push({ method, path: pathname, body });
      if (fail === "create") await json(route, 500, { error: "internal_error" });
      else if (body.role !== "household") await json(route, 400, { error: "invalid_role" });
      else if (users.some((u) => u.email === body.email.toLowerCase())) await json(route, 409, { error: "email_taken" });
      else {
        const user: MockUser = { id: `u-new-${next++}`, email: body.email.toLowerCase(), role: "household", active: true, verified: false, created_at: "2026-09-30T00:00:00Z" };
        users.push(user);
        await json(route, 201, user);
      }
      return true;
    }
    const action = /^\/users\/([^/]+)\/(invite|deactivate|activate)$/.exec(pathname);
    if (action && method === "POST") {
      const [, id, name] = action;
      writes.push({ method, path: pathname, body: undefined });
      const user = users.find((u) => u.id === id);
      if (!user) await json(route, 404, { error: "not_found" });
      else if (name === "invite") {
        if (fail === "invite") await json(route, 429, { error: "rate_limited" });
        else if (!user.active) await json(route, 409, { error: "user_inactive" });
        else if (user.verified) await json(route, 409, { error: "already_verified" });
        else await json(route, 204);
      } else if (name === "deactivate") {
        if (user.id === OWNER_USER.id) await json(route, 409, { error: "cannot_change_self" });
        else {
          user.active = false;
          await json(route, 204);
        }
      } else {
        user.active = true;
        await json(route, 204);
      }
      return true;
    }
    return false;
  }

  return { handle, users, writes };
}
