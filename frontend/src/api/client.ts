import { tokenStore, type TokenPair } from "./tokens";

export const API_URL: string = import.meta.env.VITE_API_URL || "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
  ) {
    super(code);
    this.name = "ApiError";
  }
}

export interface ApiClientOptions {
  baseUrl: string;
  fetchFn?: typeof fetch;
  store?: {
    getAccess(): string | null;
    getRefresh(): string | null;
    set(pair: TokenPair): void;
    clear(): void;
  };
  /** Called when the session cannot be recovered (refresh failed). */
  onSessionExpired?: () => void;
}

export interface RequestOptions {
  method?: string;
  body?: unknown;
  /** Skip the Authorization header and the refresh-on-401 logic. */
  anonymous?: boolean;
}

export function createApiClient(opts: ApiClientOptions) {
  const fetchFn = opts.fetchFn ?? ((...args: Parameters<typeof fetch>) => fetch(...args));
  const store = opts.store ?? tokenStore;
  let refreshing: Promise<boolean> | null = null;

  async function send(path: string, o: RequestOptions): Promise<Response> {
    const headers: Record<string, string> = {};
    if (o.body !== undefined) headers["Content-Type"] = "application/json";
    const access = store.getAccess();
    if (!o.anonymous && access) headers["Authorization"] = `Bearer ${access}`;
    return fetchFn(`${opts.baseUrl}${path}`, {
      method: o.method ?? (o.body !== undefined ? "POST" : "GET"),
      headers,
      body: o.body !== undefined ? JSON.stringify(o.body) : undefined,
    });
  }

  // Single-flight: concurrent 401s share one refresh call, since refresh tokens rotate.
  function refresh(): Promise<boolean> {
    if (refreshing) return refreshing;
    refreshing = (async () => {
      const refreshToken = store.getRefresh();
      if (!refreshToken) return false;
      try {
        const res = await fetchFn(`${opts.baseUrl}/auth/refresh`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        if (!res.ok) return false;
        store.set((await res.json()) as TokenPair);
        return true;
      } catch {
        return false;
      }
    })().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  async function parse<T>(res: Response): Promise<T> {
    if (!res.ok) {
      let code = "unknown_error";
      try {
        code = ((await res.json()) as { error?: string }).error ?? code;
      } catch {
        // Non-JSON error body.
      }
      throw new ApiError(res.status, code);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  async function request<T>(path: string, o: RequestOptions = {}): Promise<T> {
    let res = await send(path, o);
    if (res.status === 401 && !o.anonymous) {
      if (await refresh()) {
        res = await send(path, o);
      }
      if (res.status === 401) {
        store.clear();
        opts.onSessionExpired?.();
      }
    }
    return parse<T>(res);
  }

  return { request, refresh };
}

export const api = createApiClient({
  baseUrl: API_URL,
  onSessionExpired: () => window.dispatchEvent(new Event("auth:expired")),
});
