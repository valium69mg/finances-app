import { ApiError, api } from "./client";
import { tokenStore, type Role, type TokenPair } from "./tokens";

export type { Role } from "./tokens";

export interface Me {
  id: string;
  email: string;
  verified: boolean;
  role: Role;
}

/**
 * Outcome of step one of the login.
 * - `password_required`: the account exists and is verified.
 * - `verification_sent`: the account is unverified or unknown (the API answers the
 *   same for both; a verification email is sent only for real unverified accounts).
 */
export type IdentifyStatus = "password_required" | "verification_sent";

const IDENTIFY_STATUSES: readonly string[] = ["password_required", "verification_sent"];

export async function identify(email: string): Promise<IdentifyStatus> {
  const body = await api.request<{ status?: unknown }>("/auth/identify", {
    body: { email },
    anonymous: true,
  });
  if (typeof body?.status !== "string" || !IDENTIFY_STATUSES.includes(body.status)) {
    throw new ApiError(200, "unexpected_response");
  }
  return body.status as IdentifyStatus;
}

/** Step two: authenticates and stores the session. Rejects with ApiError on 401/429/400. */
export async function login(email: string, password: string): Promise<void> {
  const body = await api.request<Partial<TokenPair>>("/auth/login", {
    body: { email, password },
    anonymous: true,
  });
  if (!body?.access_token || !body.refresh_token) {
    throw new ApiError(200, "unexpected_response");
  }
  tokenStore.set(body as TokenPair);
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
