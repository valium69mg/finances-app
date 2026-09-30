import type { Request, Route } from "@playwright/test";
import { cycleOf } from "../src/pages/cycle";

/**
 * In-memory stand-in for the /future-expenses endpoints, used by helpers.ts. It keeps what the backend
 * guarantees: savings are per item, the free balance is what no item owns, paying releases exactly the saved
 * amount (the remainder returns to the free balance), paying twice is a 409 and deleting frees the savings.
 * All examples are fake: the repository is public.
 */

export interface MockFutureExpense {
  id: number;
  name: string;
  target_amount: string;
  due_date: string;
  /** Savings linked to the item. */
  saved?: string;
  status?: "active" | "paid";
  paid_at?: string | null;
  amount_paid?: string | null;
}

export type FutureExpensesFail = "list" | "save" | "pay";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Settings = any;
type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

export interface FutureExpensesDeps {
  /** Adds a Gasto to the shared in-memory expenses and returns it. */
  recordExpense: (body: Record<string, unknown>) => { id: number; amount_mxn: string };
  today: () => string;
  settings: () => Settings;
  json: Json;
}

interface Stored {
  id: number;
  name: string;
  targetCents: number;
  due_date: string;
  savedCents: number;
  status: "active" | "paid";
  paid_at: string | null;
  amountPaidCents: number | null;
  expense_movement_id: number | null;
}

const cents = (v: unknown) => Math.round(Number(v) * 100);
const money = (c: number) => (c / 100).toFixed(2);
const validAmount = (v: unknown) => typeof v === "string" && /^\d{1,12}(\.\d{1,2})?$/.test(v) && cents(v) > 0;
const monthIndex = (label: string) => Number(label.slice(0, 4)) * 12 + Number(label.slice(5, 7));

export function createFutureExpensesMock(seed: MockFutureExpense[], initialFreeBalance: string, fail: FutureExpensesFail | undefined, deps: FutureExpensesDeps) {
  let nextId = seed.reduce((max, s) => Math.max(max, s.id), 0) + 1;
  let freeCents = cents(initialFreeBalance);
  const items: Stored[] = seed.map((s) => ({
    id: s.id,
    name: s.name,
    targetCents: cents(s.target_amount),
    due_date: s.due_date,
    savedCents: cents(s.saved ?? "0"),
    status: s.status ?? "active",
    paid_at: s.paid_at ?? null,
    amountPaidCents: s.amount_paid ? cents(s.amount_paid) : null,
    expense_movement_id: null,
  }));
  /** Every write the page sent: method, path and JSON body. */
  const writes: { method: string; path: string; body: unknown }[] = [];

  const startDay = () => deps.settings().general.cycle_start_day ?? 0;

  const dto = (f: Stored) => {
    const paid = f.status === "paid";
    const cycles = Math.max(1, monthIndex(cycleOf(f.due_date, startDay())) - monthIndex(cycleOf(deps.today(), startDay())));
    const remaining = paid ? 0 : Math.max(0, f.targetCents - f.savedCents);
    return {
      id: f.id,
      name: f.name,
      target_amount: money(f.targetCents),
      due_date: f.due_date,
      status: f.status,
      saved: money(paid ? 0 : f.savedCents),
      remaining: money(remaining),
      suggested_monthly: money(paid ? 0 : Math.ceil(remaining / cycles)),
      cycles_left: paid ? 0 : cycles,
      paid_at: f.paid_at,
      amount_paid: f.amountPaidCents === null ? null : money(f.amountPaidCents),
      expense_movement_id: f.expense_movement_id,
      created_at: "2026-09-01T12:00:00Z",
      updated_at: "2026-09-01T12:00:00Z",
    };
  };

  const active = () => items.filter((f) => f.status === "active").sort((a, b) => (a.due_date === b.due_date ? a.id - b.id : a.due_date < b.due_date ? -1 : 1));

  /** The plan of the active items, as GET /dashboard embeds it. */
  function dashboard() {
    const list = active().map(dto);
    const sum = (key: "target_amount" | "saved" | "remaining" | "suggested_monthly") => money(list.reduce((t, i) => t + cents(i[key]), 0));
    return {
      items: list.map((i) => ({ id: i.id, name: i.name, due_date: i.due_date, target: i.target_amount, saved: i.saved, remaining: i.remaining, suggested_monthly: i.suggested_monthly, cycles_left: i.cycles_left })),
      target: sum("target_amount"),
      saved: sum("saved"),
      remaining: sum("remaining"),
      suggested_monthly: sum("suggested_monthly"),
      free_balance: money(freeCents),
    };
  }

  function validate(body: Record<string, unknown>): string | null {
    if (typeof body.name !== "string" || body.name.trim() === "") return "invalid future expense: name is required";
    if (!validAmount(body.target_amount)) return "invalid future expense: target amount must be greater than zero";
    if (typeof body.due_date !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(body.due_date)) return "invalid future expense: due date must be a date like YYYY-MM-DD";
    return null;
  }

  async function handle(route: Route, request: Request, pathname: string): Promise<boolean> {
    if (!pathname.startsWith("/future-expenses")) return false;
    const { json } = deps;
    const method = request.method();

    if (pathname === "/future-expenses" && method === "GET") {
      if (fail === "list") return json(route, 500, { error: "internal_error" }).then(() => true);
      const plan = dashboard();
      const paid = items.filter((f) => f.status === "paid").sort((a, b) => ((b.paid_at ?? "") < (a.paid_at ?? "") ? -1 : 1));
      await json(route, 200, {
        active: active().map(dto),
        paid: paid.map(dto),
        totals: { target: plan.target, saved: plan.saved, remaining: plan.remaining, suggested_monthly: plan.suggested_monthly },
        free_balance: plan.free_balance,
      });
      return true;
    }

    if (pathname === "/future-expenses" && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      const problem = fail === "save" ? "invalid future expense: name is required" : validate(body);
      if (problem) return json(route, 400, { error: "invalid_future_expense", message: problem }).then(() => true);
      const item: Stored = {
        id: nextId++,
        name: body.name.trim(),
        targetCents: cents(body.target_amount),
        due_date: body.due_date,
        savedCents: 0,
        status: "active",
        paid_at: null,
        amountPaidCents: null,
        expense_movement_id: null,
      };
      items.push(item);
      await json(route, 201, dto(item));
      return true;
    }

    const m = /^\/future-expenses\/(\d+)(\/savings|\/assign|\/pay)?$/.exec(pathname);
    if (!m) return false;
    const item = items.find((f) => f.id === Number(m[1]));
    const action = m[2];

    if (!action && method === "PUT") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (!item) return json(route, 404, { error: "not_found" }).then(() => true);
      if (item.status === "paid") return json(route, 409, { error: "already_paid", message: "future expense is already paid" }).then(() => true);
      const problem = fail === "save" ? "invalid future expense: name is required" : validate(body);
      if (problem) return json(route, 400, { error: "invalid_future_expense", message: problem }).then(() => true);
      item.name = body.name.trim();
      item.targetCents = cents(body.target_amount);
      item.due_date = body.due_date;
      await json(route, 200, dto(item));
      return true;
    }

    if (!action && method === "DELETE") {
      writes.push({ method, path: pathname, body: undefined });
      if (!item) return json(route, 404, { error: "not_found" }).then(() => true);
      // The linked savings are unlinked, never deleted: they become free balance.
      if (item.status === "active") freeCents += item.savedCents;
      items.splice(items.indexOf(item), 1);
      await json(route, 204);
      return true;
    }

    if (method !== "POST") return false;
    const body = request.postDataJSON() ?? {};
    writes.push({ method, path: pathname, body });
    if (!item) return json(route, 404, { error: "not_found" }).then(() => true);
    if (item.status === "paid") return json(route, 409, { error: "already_paid", message: "future expense is already paid" }).then(() => true);

    if (action === "/savings" || action === "/assign") {
      if (!validAmount(body.amount)) return json(route, 400, { error: "invalid_future_expense", message: "invalid future expense: amount must be greater than zero" }).then(() => true);
      const amount = cents(body.amount);
      if (action === "/assign") {
        if (amount > freeCents) return json(route, 409, { error: "insufficient_free_balance", message: "not enough free balance" }).then(() => true);
        freeCents -= amount;
      }
      item.savedCents += amount;
      const planned = dto(item);
      if (action === "/assign") {
        await json(route, 200, planned);
      } else {
        await json(route, 201, { item: planned, saving: { id: 9000 + item.id, date: body.date ?? deps.today(), description: `Ahorro para ${item.name}`, category: "Gastos futuros", amount: money(amount), amount_mxn: money(amount) } });
      }
      return true;
    }

    if (action === "/pay") {
      if (fail === "pay") return json(route, 500, { error: "internal_error" }).then(() => true);
      const paidCents = body.amount === undefined ? item.targetCents : cents(body.amount);
      if (body.amount !== undefined && !validAmount(body.amount)) return json(route, 400, { error: "invalid_future_expense", message: "invalid future expense: amount must be greater than zero" }).then(() => true);
      const category = body.category || "Otros";
      const isExpenseCategory = deps.settings().categories.some((c: { name: string; kind: string }) => c.name === category && c.kind === "Gasto");
      if (!isExpenseCategory) return json(route, 400, { error: "invalid_movement", message: `invalid movement: "${category}" is not a valid Gasto category` }).then(() => true);
      const date = body.date ?? deps.today();
      const expense = deps.recordExpense({ date, description: item.name, category, currency: "MXN", amount: money(paidCents) });
      // What was saved is released; the part the payment did not use returns to the free balance.
      freeCents += Math.max(item.savedCents - paidCents, 0);
      item.status = "paid";
      item.paid_at = date;
      item.amountPaidCents = paidCents;
      item.expense_movement_id = expense.id;
      await json(route, 200, { item: dto(item), expense: { id: expense.id, date, description: item.name, category, amount: money(paidCents), amount_mxn: expense.amount_mxn } });
      return true;
    }
    return false;
  }

  return { handle, dashboard, writes, freeBalance: () => money(freeCents) };
}
