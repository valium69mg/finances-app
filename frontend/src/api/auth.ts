import { api } from "./client";
import { tokenStore, type TokenPair } from "./tokens";

export interface Me {
  id: string;
  email: string;
  verified: boolean;
}

export type LoginResult = { kind: "session" } | { kind: "verification_pending" };

// Bypass the wrapper for login: the API answers 200 with tokens or 202 pending,
// and the wrapper only exposes the parsed body, so we distinguish by shape.
export async function login(email: string, password: string): Promise<LoginResult> {
  const body = await api.request<TokenPair | { status: string }>("/auth/login", {
    body: { email, password },
    anonymous: true,
  });
  if ("access_token" in body) {
    tokenStore.set(body);
    return { kind: "session" };
  }
  return { kind: "verification_pending" };
}

export async function logout(): Promise<void> {
  const refreshToken = tokenStore.getRefresh();
  tokenStore.clear();
  if (!refreshToken) return;
  try {
    await api.request<void>("/auth/logout", { body: { refresh_token: refreshToken }, anonymous: true });
  } catch {
    // Local session is already cleared; server-side revocation is best effort.
  }
}

export function verifyEmail(token: string, password: string): Promise<void> {
  return api.request<void>("/auth/verify", { body: { token, password }, anonymous: true });
}

export function fetchMe(): Promise<Me> {
  return api.request<Me>("/auth/me");
}
