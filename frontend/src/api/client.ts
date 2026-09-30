import { tokenStore, type TokenPair } from "./tokens";

export const API_URL: string = import.meta.env.VITE_API_URL || "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    /** Human-readable detail some endpoints add next to the code (e.g. invalid_settings). */
    public message: string = code,
  ) {
    super(code);
    this.name = "ApiError";
  }
}

type RefreshOutcome = "ok" | "invalid" | "unavailable";

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
  /** Read a successful response as a Blob (file download) instead of JSON. */
  blob?: boolean;
}

export function createApiClient(opts: ApiClientOptions) {
  const fetchFn = opts.fetchFn ?? ((...args: Parameters<typeof fetch>) => fetch(...args));
  const store = opts.store ?? tokenStore;
  let refreshing: Promise<RefreshOutcome> | null = null;

  async function send(path: string, o: RequestOptions): Promise<Response> {
    const headers: Record<string, string> = {};
    // A FormData body (file upload) is sent as is: the browser adds the multipart boundary itself.
    const isForm = typeof FormData !== "undefined" && o.body instanceof FormData;
    if (o.body !== undefined && !isForm) headers["Content-Type"] = "application/json";
    const access = store.getAccess();
    if (!o.anonymous && access) headers["Authorization"] = `Bearer ${access}`;
    return fetchFn(`${opts.baseUrl}${path}`, {
      method: o.method ?? (o.body !== undefined ? "POST" : "GET"),
      headers,
      body: o.body === undefined ? undefined : isForm ? (o.body as FormData) : JSON.stringify(o.body),
    });
  }

  // Single-flight: concurrent 401s share one refresh call, since refresh tokens rotate.
  // "invalid" means the server rejected the token (the session is over); "unavailable" means
  // the answer says nothing about the token (rate limit, server error, network), so the
  // session must survive and the call simply fails.
  function refreshOutcome(): Promise<RefreshOutcome> {
    if (refreshing) return refreshing;
    refreshing = (async (): Promise<RefreshOutcome> => {
      const refreshToken = store.getRefresh();
      if (!refreshToken) return "invalid";
      try {
        const res = await fetchFn(`${opts.baseUrl}/auth/refresh`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        if (res.ok) {
          store.set((await res.json()) as TokenPair);
          return "ok";
        }
        return res.status === 429 || res.status >= 500 ? "unavailable" : "invalid";
      } catch {
        return "unavailable";
      }
    })().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  async function refresh(): Promise<boolean> {
    return (await refreshOutcome()) === "ok";
  }

  async function parse<T>(res: Response, blob = false): Promise<T> {
    if (!res.ok) {
      let code = "unknown_error";
      let message: string | undefined;
      try {
        const body = (await res.json()) as { error?: string; message?: string };
        code = body.error ?? code;
        message = typeof body.message === "string" ? body.message : undefined;
      } catch {
        // Non-JSON error body.
      }
      throw new ApiError(res.status, code, message);
    }
    if (res.status === 204) return undefined as T;
    if (blob) return (await res.blob()) as T;
    return (await res.json()) as T;
  }

  async function request<T>(path: string, o: RequestOptions = {}): Promise<T> {
    let res = await send(path, o);
    if (res.status === 401 && !o.anonymous) {
      const outcome = await refreshOutcome();
      if (outcome === "ok") {
        res = await send(path, o);
      }
      if (res.status === 401 && outcome !== "unavailable") {
        store.clear();
        opts.onSessionExpired?.();
      }
    }
    return parse<T>(res, o.blob);
  }

  return { request, refresh };
}

export const api = createApiClient({
  baseUrl: API_URL,
  onSessionExpired: () => window.dispatchEvent(new Event("auth:expired")),
});
