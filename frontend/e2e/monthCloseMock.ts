import type { Request, Route } from "@playwright/test";

/** In-memory stand-in for the /month-close endpoints, used by helpers.ts. */

export interface MockClose {
  period: string;
  closed_at: string | null;
  categories: { category: string; spent: string; budget: string | null; remaining: string | null; over_budget: boolean }[];
  income: string;
  expenses: string;
  savings: string;
  available: string;
  emergency: { accumulated: string; goal: string };
  suggestion: { to_emergency_fund: string; to_investments: string; to_future_expenses: string; investments_paused: boolean } | null;
  adjustments: { category: string; kind: "Gasto" | "Ahorro"; budget: string; real: string; deviation_pct: string }[];
  tax_filing_status: "ninguna" | "pendiente" | "pagada" | null;
}

export type MonthCloseFail = "preview" | "incomplete" | "list" | "create" | "delete";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Settings = any;
type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

/** What the shared in-memory movements say about a month, as GET /dashboard would. */
export interface MonthFigures {
  categories: MockClose["categories"];
  income: string;
  expenses: string;
  savings: string;
  available: string;
  emergency: { accumulated: string; goal: string };
}

export interface MonthCloseDeps {
  figures: (month: string) => MonthFigures;
  filingStatus: (month: string) => "ninguna" | "pendiente" | "pagada";
  today: () => string;
  settings: () => Settings;
  json: Json;
}

const money = (n: number) => n.toFixed(2);

function previousMonthOf(month: string) {
  const [y, m] = month.split("-").map(Number);
  return m === 1 ? `${y - 1}-12` : `${y}-${String(m - 1).padStart(2, "0")}`;
}

export function createMonthCloseMock(seed: MockClose[], fail: MonthCloseFail | undefined, deps: MonthCloseDeps) {
  const closes: MockClose[] = structuredClone(seed);
  /** Every write the page sent: method, path and JSON body. */
  const writes: { method: string; path: string; body: unknown }[] = [];

  /** Same rules as the backend: budget-vs-real hints above 20%, and emergency fund first, then Inversiones or Gastos futuros while paused. */
  function compute(period: string): MockClose {
    const f = deps.figures(period);
    const paused: boolean = (deps.settings().investment_pause?.months ?? []).includes(period);
    const adjustments = f.categories
      .filter((c) => c.budget !== null && Number(c.budget) !== 0)
      .filter((c) => Math.abs(Number(c.spent) - Number(c.budget)) * 5 > Math.abs(Number(c.budget)))
      .map((c) => ({
        category: c.category,
        kind: "Gasto" as const,
        budget: c.budget as string,
        real: c.spent,
        deviation_pct: money(((Number(c.spent) - Number(c.budget)) * 100) / Number(c.budget)),
      }));
    const available = Number(f.available);
    let suggestion: MockClose["suggestion"] = null;
    if (available > 0) {
      const room = Math.max(0, Number(f.emergency.goal) - Number(f.emergency.accumulated));
      const toFund = Math.min(available, room);
      const rest = available - toFund;
      suggestion = {
        to_emergency_fund: money(toFund),
        to_investments: paused ? "0" : money(rest),
        to_future_expenses: paused ? money(rest) : "0",
        investments_paused: paused,
      };
    }
    return { ...f, period, closed_at: null, suggestion, adjustments, tax_filing_status: deps.filingStatus(period) };
  }

  const valid = (period: string) => /^\d{4}-(0[1-9]|1[0-2])$/.test(period);
  const sorted = () => [...closes].sort((a, b) => (a.period < b.period ? 1 : -1));

  async function handle(route: Route, request: Request, pathname: string, params: URLSearchParams): Promise<boolean> {
    if (!pathname.startsWith("/month-close")) return false;
    const { json } = deps;
    const method = request.method();

    if (pathname === "/month-close/preview" && method === "GET") {
      if (fail === "incomplete") return json(route, 422, { error: "settings_incomplete", message: "missing required config: emergency_months" }).then(() => true);
      if (fail === "preview") return json(route, 500, { error: "internal_error" }).then(() => true);
      const period = params.get("period") || previousMonthOf(deps.today().slice(0, 7));
      if (!valid(period)) {
        await json(route, 400, { error: "invalid_close", message: `invalid month close: period "${period}" must be YYYY-MM` });
        return true;
      }
      await json(route, 200, { preview: compute(period), existing: closes.find((c) => c.period === period) ?? null });
      return true;
    }

    if (pathname === "/month-close" && method === "GET") {
      if (fail === "list") return json(route, 500, { error: "internal_error" }).then(() => true);
      await json(route, 200, sorted());
      return true;
    }

    if (pathname === "/month-close" && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "create") return json(route, 500, { error: "internal_error" }).then(() => true);
      const period: string = body.period;
      if (!valid(period) || period > deps.today().slice(0, 7)) {
        await json(route, 400, { error: "invalid_close", message: `invalid month close: period ${period} is in the future` });
        return true;
      }
      if (closes.some((c) => c.period === period)) {
        await json(route, 409, { error: "already_closed", message: `month already closed: ${period}` });
        return true;
      }
      // The snapshot is frozen here: later edits to the mock movements never reach it.
      const close = { ...compute(period), closed_at: new Date().toISOString() };
      closes.push(close);
      await json(route, 201, close);
      return true;
    }

    const m = /^\/month-close\/(\d{4}-\d{2})$/.exec(pathname);
    if (!m) return false;
    const close = closes.find((c) => c.period === m[1]);

    if (method === "GET") {
      if (!close) return json(route, 404, { error: "not_found" }).then(() => true);
      await json(route, 200, close);
      return true;
    }
    if (method === "DELETE") {
      writes.push({ method, path: pathname, body: undefined });
      if (fail === "delete") return json(route, 500, { error: "internal_error" }).then(() => true);
      if (!close) return json(route, 404, { error: "not_found" }).then(() => true);
      closes.splice(closes.indexOf(close), 1);
      await json(route, 204);
      return true;
    }
    return false;
  }

  return { closes, writes, handle };
}
