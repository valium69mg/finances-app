import type { Request, Route } from "@playwright/test";
import { cycleOf } from "../src/pages/cycle";

/**
 * In-memory stand-in for the /expense-requests endpoints, used by helpers.ts. It keeps what the backend
 * guarantees: a household user sees only her own requests, only the requester cancels and only while the request
 * is pending, decided requests are immutable (409 invalid_state) except for the owner's revert of an approval, approving as a Gasto registers the expense
 * (a request that does not fit its budget is still approved) and approving as a future expense creates the item
 * without touching the budget. All examples are fake: the repository is public.
 */

export interface MockExpenseRequest {
  id: number;
  requester_email?: string;
  amount: string;
  description: string;
  suggested_category?: string | null;
  expense_date?: string;
  status?: "solicitada" | "aprobada" | "rechazada" | "cancelada";
  decision_comment?: string | null;
  result_kind?: "gasto" | "gasto_futuro" | null;
  /** The Gasto or the future expense an approved request points to (seeded by the test). */
  result_movement_id?: number | null;
  result_future_expense_id?: number | null;
  revert_count?: number;
}

export type ExpenseRequestsFail = "list" | "create" | "rate_limit" | "approve";

/** The signed-in household user of the mock (GET /auth/me answers with it). */
export const HOUSEHOLD_EMAIL = "her@example.com";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Settings = any;
type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

interface Stored {
  id: number;
  requester_email: string;
  amount: string;
  description: string;
  suggested_category: string | null;
  expense_date: string;
  status: "solicitada" | "aprobada" | "rechazada" | "cancelada";
  decision_comment: string | null;
  decided_at: string | null;
  result_kind: "gasto" | "gasto_futuro" | null;
  result_movement_id: number | null;
  result_future_expense_id: number | null;
  created_at: string;
  revert_count: number;
  reverted_at: string | null;
}

export interface ExpenseRequestsDeps {
  role: "owner" | "household";
  today: () => string;
  settings: () => Settings;
  /** The shared in-memory Gastos, to compute what is spent in a cycle. */
  expenses: () => { date: string; category: string; amount_mxn: string }[];
  /** Adds a Gasto to the shared in-memory expenses and returns its id. */
  recordExpense: (body: Record<string, unknown>) => { id: number; amount_mxn: string };
  /** Adds an active future expense and returns its id. */
  recordFuture: (name: string, target: string, due: string) => number;
  /** Deletes a Gasto of the shared in-memory expenses (a missing one is fine). */
  removeExpense: (id: number) => void;
  /** Deletes an active future expense; a paid one is refused and a missing one is fine. */
  removeFuture: (id: number) => "removed" | "paid" | "missing";
  json: Json;
}

const cents = (v: unknown) => Math.round(Number(v) * 100);
const money = (c: number) => (c / 100).toFixed(2);
const validAmount = (v: unknown) => typeof v === "string" && /^\d{1,12}(\.\d{1,2})?$/.test(v) && cents(v) > 0;
const isDate = (v: unknown) => typeof v === "string" && /^\d{4}-\d{2}-\d{2}$/.test(v);

export function createExpenseRequestsMock(seed: MockExpenseRequest[], fail: ExpenseRequestsFail | undefined, deps: ExpenseRequestsDeps) {
  const requests: Stored[] = seed.map((s) => ({
    id: s.id,
    requester_email: s.requester_email ?? HOUSEHOLD_EMAIL,
    amount: s.amount,
    description: s.description,
    suggested_category: s.suggested_category ?? null,
    expense_date: s.expense_date ?? "2026-10-03",
    status: s.status ?? "solicitada",
    decision_comment: s.decision_comment ?? null,
    decided_at: s.status && s.status !== "solicitada" ? "2026-10-04T12:00:00Z" : null,
    result_kind: s.result_kind ?? null,
    result_movement_id: s.result_movement_id ?? null,
    result_future_expense_id: s.result_future_expense_id ?? null,
    created_at: `2026-10-0${Math.min(9, s.id)}T12:00:00Z`,
    revert_count: s.revert_count ?? 0,
    reverted_at: s.revert_count ? "2026-10-05T12:00:00Z" : null,
  }));
  let nextId = seed.reduce((max, s) => Math.max(max, s.id), 0) + 1;
  /** Every write the page sent: method, path and JSON body. */
  const writes: { method: string; path: string; body: unknown }[] = [];
  /** Every budget check the dialog asked for: category and date. */
  const budgetChecks: { id: number; category: string; date: string }[] = [];

  const me = () => (deps.role === "household" ? HOUSEHOLD_EMAIL : "admin@example.com");
  const gastoCategories = () =>
    deps
      .settings()
      .categories.filter((c: { kind: string }) => ["Gasto", "fixed", "variable"].includes(c.kind))
      .map((c: { name: string }) => c.name) as string[];

  const dto = (r: Stored) => ({ ...r });

  /** Budget and spending of a category in the cycle that contains `date`, as the backend resolves them. */
  function budgetCheck(r: Stored, category: string, date: string) {
    const startDay = deps.settings().general.cycle_start_day ?? 0;
    const label = cycleOf(date, startDay);
    const cat = deps.settings().categories.find((c: { name: string }) => c.name === category);
    const spent = deps
      .expenses()
      .filter((e) => e.category === category && cycleOf(e.date, startDay) === label)
      .reduce((total, e) => total + cents(e.amount_mxn), 0);
    const amount = cents(r.amount);
    const budget: number | null = cat && cat.budget !== null ? cents(cat.budget) : null;
    if (budget === null || budget <= 0) {
      return { category, budget: null, spent: money(spent), remaining: null, amount: money(amount), projected_spent: money(spent + amount), projected_remaining: null, fits: null, over_by: "0.00" };
    }
    const projected = spent + amount;
    return {
      category,
      budget: money(budget),
      spent: money(spent),
      remaining: money(budget - spent),
      amount: money(amount),
      projected_spent: money(projected),
      projected_remaining: money(budget - projected),
      fits: projected <= budget,
      over_by: money(Math.max(0, projected - budget)),
    };
  }

  async function handle(route: Route, request: Request, pathname: string, searchParams: URLSearchParams): Promise<boolean> {
    if (!pathname.startsWith("/expense-requests")) return false;
    const { json } = deps;
    const method = request.method();
    const reply = async (status: number, body?: unknown) => {
      await json(route, status, body);
      return true;
    };

    if (pathname === "/expense-requests" && method === "GET") {
      if (fail === "list") return reply(500, { error: "internal_error" });
      const status = searchParams.get("status");
      const rows = requests
        .filter((r) => deps.role === "owner" || r.requester_email === me())
        .filter((r) => !status || r.status === status)
        .sort((a, b) => b.id - a.id);
      return reply(200, rows.map(dto));
    }

    if (pathname === "/expense-requests/categories" && method === "GET") return reply(200, gastoCategories());

    if (pathname === "/expense-requests" && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "rate_limit") return reply(429, { error: "rate_limited" });
      if (fail === "create") return reply(400, { error: "invalid_expense_request", message: "invalid expense request: amount must be greater than zero" });
      if (!validAmount(body.amount)) return reply(400, { error: "invalid_expense_request", message: "invalid expense request: amount must be greater than zero" });
      if (typeof body.description !== "string" || body.description.trim() === "") return reply(400, { error: "invalid_expense_request", message: "invalid expense request: description is required" });
      if (body.suggested_category && !gastoCategories().includes(body.suggested_category)) {
        return reply(400, { error: "invalid_expense_request", message: "invalid expense request: suggested category must be one of the Gasto categories" });
      }
      const row: Stored = {
        id: nextId++,
        requester_email: me(),
        amount: body.amount,
        description: body.description.trim(),
        suggested_category: body.suggested_category || null,
        expense_date: isDate(body.date) ? body.date : deps.today(),
        status: "solicitada",
        decision_comment: null,
        decided_at: null,
        result_kind: null,
        result_movement_id: null,
        result_future_expense_id: null,
        created_at: "2026-10-05T12:00:00Z",
        revert_count: 0,
        reverted_at: null,
      };
      requests.push(row);
      return reply(201, dto(row));
    }

    const match = /^\/expense-requests\/(\d+)\/(cancel|budget-check|approve|reject|revert)$/.exec(pathname);
    if (!match) return false;
    const id = Number(match[1]);
    const action = match[2];
    const row = requests.find((r) => r.id === id);

    if (action === "budget-check" && method === "GET") {
      if (!row) return reply(404, { error: "not_found" });
      const category = searchParams.get("category") ?? "";
      const date = searchParams.get("date") || row.expense_date;
      budgetChecks.push({ id, category, date });
      if (!gastoCategories().includes(category)) return reply(400, { error: "invalid_expense_request", message: "invalid expense request: category must be one of the Gasto categories" });
      return reply(200, budgetCheck(row, category, date));
    }
    if (method !== "POST") return false;
    const body = action === "cancel" ? undefined : request.postDataJSON();
    writes.push({ method, path: pathname, body });
    if (!row) return reply(404, { error: "not_found" });

    if (action === "cancel") {
      if (row.requester_email !== me()) return reply(403, { error: "forbidden" });
      if (row.status !== "solicitada") return reply(409, { error: "invalid_state" });
      row.status = "cancelada";
      row.decided_at = "2026-10-05T12:00:00Z";
      return reply(200, dto(row));
    }

    if (action === "revert") {
      // Owner only (the household role never reaches here: helpers.ts answers 403). Only an approved request.
      if (row.status !== "aprobada") return reply(409, { error: "invalid_state" });
      if (row.result_kind === "gasto_futuro" && row.result_future_expense_id !== null) {
        if (deps.removeFuture(row.result_future_expense_id) === "paid") return reply(409, { error: "future_expense_paid", message: "the future expense is already paid" });
      } else if (row.result_kind === "gasto" && row.result_movement_id !== null) {
        deps.removeExpense(row.result_movement_id);
      }
      row.status = "solicitada";
      row.decided_at = null;
      row.result_kind = null;
      row.result_movement_id = null;
      row.result_future_expense_id = null;
      row.revert_count += 1;
      row.reverted_at = "2026-10-06T12:00:00Z";
      return reply(200, dto(row));
    }
    if (row.status !== "solicitada") return reply(409, { error: "invalid_state" });

    if (action === "reject") {
      if (typeof body.comment !== "string" || body.comment.trim() === "") return reply(400, { error: "invalid_expense_request", message: "invalid expense request: comment is required" });
      row.status = "rechazada";
      row.decision_comment = body.comment.trim();
      row.decided_at = "2026-10-05T12:00:00Z";
      return reply(200, dto(row));
    }

    // approve
    if (fail === "approve") return reply(500, { error: "internal_error" });
    if (body.destination === "gasto") {
      if (!gastoCategories().includes(body.category)) return reply(400, { error: "invalid_expense_request", message: "invalid expense request: category must be one of the Gasto categories" });
      const date = isDate(body.date) ? body.date : row.expense_date;
      const expense = deps.recordExpense({ date, description: row.description, category: body.category, amount: row.amount, payment_method: body.payment_method });
      row.status = "aprobada";
      row.result_kind = "gasto";
      row.result_movement_id = expense.id;
      row.decided_at = "2026-10-05T12:00:00Z";
      // The feedback of the expenses module, after the Gasto exists.
      const after = budgetCheck({ ...row, amount: "0" }, body.category, date);
      return reply(200, {
        request: dto(row),
        budget: {
          month: cycleOf(date, deps.settings().general.cycle_start_day ?? 0),
          category: body.category,
          budget: after.budget,
          spent: after.spent,
          remaining: after.remaining,
          over_budget: after.budget !== null && cents(after.spent) > cents(after.budget),
        },
      });
    }
    if (body.destination === "gasto_futuro") {
      if (!isDate(body.due_date)) return reply(400, { error: "invalid_future_expense", message: "invalid future expense: due date must be a date like YYYY-MM-DD" });
      row.status = "aprobada";
      row.result_kind = "gasto_futuro";
      row.result_future_expense_id = deps.recordFuture(row.description, row.amount, body.due_date);
      row.decided_at = "2026-10-05T12:00:00Z";
      return reply(200, { request: dto(row), budget: null });
    }
    return reply(400, { error: "invalid_expense_request", message: "invalid expense request: destination must be gasto or gasto_futuro" });
  }

  return { handle, requests, writes, budgetChecks };
}
