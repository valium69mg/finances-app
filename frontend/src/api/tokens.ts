const ACCESS_KEY = "access_token";
const REFRESH_KEY = "refresh_token";
const ROLE_KEY = "role";

/** What the account may do. The backend enforces it on every route; the UI only shapes itself with it. */
export type Role = "owner" | "household";

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  /** Sent by login and refresh; absent in sessions stored before roles existed. */
  role?: string;
}

/** Fired on every change of the stored session so the auth context can follow it. */
export const SESSION_EVENT = "auth:session";

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function notify() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(SESSION_EVENT));
}

const isRole = (value: unknown): value is Role => value === "owner" || value === "household";

export const tokenStore = {
  getAccess: () => read(ACCESS_KEY),
  getRefresh: () => read(REFRESH_KEY),
  /** The stored role, or null for a session that predates roles (the next refresh brings it). */
  getRole: (): Role | null => {
    const role = read(ROLE_KEY);
    return isRole(role) ? role : null;
  },
  set(pair: TokenPair) {
    try {
      localStorage.setItem(ACCESS_KEY, pair.access_token);
      localStorage.setItem(REFRESH_KEY, pair.refresh_token);
      if (isRole(pair.role)) localStorage.setItem(ROLE_KEY, pair.role);
    } catch {
      // Storage unavailable: the session will not survive a reload.
    }
    notify();
  },
  /** Updates only the role (e.g. when /auth/me says it changed). */
  setRole(role: Role) {
    if (read(ROLE_KEY) === role) return;
    try {
      localStorage.setItem(ROLE_KEY, role);
    } catch {
      // Storage unavailable: the role is read again from the next response.
    }
    notify();
  },
  clear() {
    try {
      localStorage.removeItem(ACCESS_KEY);
      localStorage.removeItem(REFRESH_KEY);
      localStorage.removeItem(ROLE_KEY);
    } catch {
      // Nothing to clear.
    }
    notify();
  },
};
