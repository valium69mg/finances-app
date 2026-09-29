const ACCESS_KEY = "access_token";
const REFRESH_KEY = "refresh_token";

export interface TokenPair {
  access_token: string;
  refresh_token: string;
}

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export const tokenStore = {
  getAccess: () => read(ACCESS_KEY),
  getRefresh: () => read(REFRESH_KEY),
  set(pair: TokenPair) {
    try {
      localStorage.setItem(ACCESS_KEY, pair.access_token);
      localStorage.setItem(REFRESH_KEY, pair.refresh_token);
    } catch {
      // Storage unavailable: the session will not survive a reload.
    }
  },
  clear() {
    try {
      localStorage.removeItem(ACCESS_KEY);
      localStorage.removeItem(REFRESH_KEY);
    } catch {
      // Nothing to clear.
    }
  },
};
