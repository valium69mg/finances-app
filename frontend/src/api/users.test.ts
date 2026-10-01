import { describe, expect, it, vi } from "vitest";
import { createUsersApi } from "./users";

function setup(response: unknown = undefined) {
  const request = vi.fn(async () => response);
  return { request, api: createUsersApi({ request: request as never }) };
}

describe("users api", () => {
  it("lists the accounts", async () => {
    const { api, request } = setup([]);
    await api.list();
    expect(request).toHaveBeenCalledWith("/users");
  });

  it("creates a household account with only the email and the role", async () => {
    const { api, request } = setup();
    await api.create({ email: "her@example.com", role: "household" });
    expect(request).toHaveBeenCalledWith("/users", { method: "POST", body: { email: "her@example.com", role: "household" } });
  });

  it.each(["invite", "deactivate", "activate"] as const)("%s is a body-less POST on the user", async (action) => {
    const { api, request } = setup();
    await api[action]("u 1/2");
    expect(request).toHaveBeenCalledWith(`/users/u%201%2F2/${action}`, { method: "POST" });
  });
});
