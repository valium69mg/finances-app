import type { Request, Route } from "@playwright/test";

/** In-memory stand-in for the /bills endpoints, used by helpers.ts. */

export interface MockOccurrence {
  id: number;
  due_date: string;
  status: "pending" | "paid" | "skipped";
  paid_on: string | null;
  expense_movement_id: number | null;
  amount_paid: string | null;
  currency: string | null;
  resolved_at: string | null;
}

/** A bill as seeded by a test: history holds resolved occurrences, newest first. */
export interface MockBill {
  id: number;
  name: string;
  category: string;
  amount: string | null;
  currency: string;
  recurrence: "weekly" | "biweekly" | "monthly" | "bimonthly" | "yearly";
  next_due_date: string;
  reminder_lead_days: number;
  active: boolean;
  notes: string;
  history?: MockOccurrence[];
}

export type BillsFail = "list" | "detail" | "save" | "pay" | "skip";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Settings = any;
type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

export interface BillsDeps {
  /** Adds an expense to the shared in-memory expenses and returns it. */
  recordExpense: (body: Record<string, unknown>) => { id: number; amount_mxn: string };
  today: () => string;
  settings: () => Settings;
  json: Json;
}

interface Stored extends MockBill {
  anchor_day: number;
  pending: MockOccurrence;
  history: MockOccurrence[];
}

const DAY_MS = 86_400_000;
const utc = (date: string) => Date.parse(`${date}T00:00:00Z`);
const daysBetween = (from: string, to: string) => Math.round((utc(to) - utc(from)) / DAY_MS);
const iso = (ms: number) => new Date(ms).toISOString().slice(0, 10);

/** Same rule as the backend: month steps land on the anchor day, clamped to the month's last day. */
export function nextDue(recurrence: MockBill["recurrence"], anchorDay: number, from: string): string {
  if (recurrence === "weekly") return iso(utc(from) + 7 * DAY_MS);
  if (recurrence === "biweekly") return iso(utc(from) + 14 * DAY_MS);
  const [y, m] = from.split("-").map(Number);
  const step = recurrence === "monthly" ? 1 : recurrence === "bimonthly" ? 2 : 12;
  const first = new Date(Date.UTC(y, m - 1 + step, 1));
  const last = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate();
  return iso(Date.UTC(first.getUTCFullYear(), first.getUTCMonth(), Math.min(anchorDay, last)));
}

export function createBillsMock(seed: MockBill[], fail: BillsFail | undefined, deps: BillsDeps) {
  let nextBillId = seed.reduce((max, b) => Math.max(max, b.id), 0) + 1;
  let nextOccId = 1000;
  const newPending = (due: string): MockOccurrence => ({
    id: nextOccId++,
    due_date: due,
    status: "pending",
    paid_on: null,
    expense_movement_id: null,
    amount_paid: null,
    currency: null,
    resolved_at: null,
  });
  const bills: Stored[] = seed.map((b) => ({
    ...structuredClone(b),
    anchor_day: Number(b.next_due_date.slice(8, 10)),
    pending: newPending(b.next_due_date),
    history: structuredClone(b.history ?? []),
  }));
  /** Every write the page sent: method, path and JSON body. */
  const writes: { method: string; path: string; body: unknown }[] = [];

  const dto = (b: Stored) => {
    const today = deps.today();
    const days = daysBetween(today, b.pending.due_date);
    return {
      id: b.id,
      name: b.name,
      category: b.category,
      amount: b.amount,
      currency: b.currency,
      recurrence: b.recurrence,
      next_due_date: b.pending.due_date,
      reminder_lead_days: b.reminder_lead_days,
      active: b.active,
      notes: b.notes,
      created_at: "2026-09-01T12:00:00Z",
      pending_occurrence: b.pending,
      overdue: days < 0,
      due_soon: days >= 0 && days <= b.reminder_lead_days,
      days_until_due: days,
    };
  };

  /** Expense categories of the settings, like the backend's category validation. */
  const isExpenseCategory = (name: string) => deps.settings().categories.some((c: { name: string; kind: string }) => c.name === name && c.kind === "Gasto");

  function apply(b: Stored, body: Record<string, unknown>) {
    const due = String(body.next_due_date);
    if (due !== b.pending.due_date) {
      b.anchor_day = Number(due.slice(8, 10));
      b.pending.due_date = due;
    }
    b.name = String(body.name).trim();
    b.category = String(body.category);
    b.amount = (body.amount as string | null) ?? null;
    b.currency = String(body.currency ?? "MXN");
    b.recurrence = body.recurrence as Stored["recurrence"];
    b.reminder_lead_days = body.reminder_lead_days === undefined ? 3 : Number(body.reminder_lead_days);
    b.active = body.active === undefined ? true : Boolean(body.active);
    b.notes = String(body.notes ?? "");
  }

  async function handle(route: Route, request: Request, pathname: string, params: URLSearchParams): Promise<boolean> {
    if (!pathname.startsWith("/bills")) return false;
    const { json } = deps;
    const method = request.method();

    if (pathname === "/bills" && method === "GET") {
      if (fail === "list") return json(route, 500, { error: "internal_error" }).then(() => true);
      const all = params.get("include_inactive") === "true";
      const rows = bills
        .filter((b) => all || b.active)
        .sort((a, b) => (a.pending.due_date === b.pending.due_date ? a.name.localeCompare(b.name) : a.pending.due_date < b.pending.due_date ? -1 : 1));
      await json(route, 200, rows.map(dto));
      return true;
    }

    if (pathname === "/bills" && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "save") return json(route, 400, { error: "invalid_bill", message: "invalid bill input: name is required" }).then(() => true);
      if (!isExpenseCategory(body.category)) {
        await json(route, 400, { error: "invalid_bill", message: `invalid bill input: "${body.category}" is not an expense category` });
        return true;
      }
      const bill: Stored = {
        id: nextBillId++,
        name: "",
        category: "",
        amount: null,
        currency: "MXN",
        recurrence: "monthly",
        next_due_date: body.next_due_date,
        reminder_lead_days: 3,
        active: true,
        notes: "",
        anchor_day: Number(String(body.next_due_date).slice(8, 10)),
        pending: newPending(body.next_due_date),
        history: [],
      };
      apply(bill, body);
      bills.push(bill);
      await json(route, 201, dto(bill));
      return true;
    }

    const m = /^\/bills\/(\d+)(\/pay|\/skip)?$/.exec(pathname);
    if (!m) return false;
    const bill = bills.find((b) => b.id === Number(m[1]));

    if (!m[2] && method === "GET") {
      if (fail === "detail") return json(route, 500, { error: "internal_error" }).then(() => true);
      if (!bill) return json(route, 404, { error: "not_found" }).then(() => true);
      await json(route, 200, { ...dto(bill), history: bill.history });
      return true;
    }

    if (!m[2] && method === "PUT") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "save") return json(route, 400, { error: "invalid_bill", message: "invalid bill input: name is required" }).then(() => true);
      if (!bill) return json(route, 404, { error: "not_found" }).then(() => true);
      apply(bill, body);
      await json(route, 200, dto(bill));
      return true;
    }

    if (!m[2] && method === "DELETE") {
      writes.push({ method, path: pathname, body: undefined });
      if (!bill) return json(route, 404, { error: "not_found" }).then(() => true);
      bill.active = false;
      await json(route, 204);
      return true;
    }

    if (m[2] === "/pay" && method === "POST") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      if (fail === "pay") return json(route, 500, { error: "internal_error" }).then(() => true);
      if (!bill) return json(route, 404, { error: "not_found" }).then(() => true);
      if (!bill.active) return json(route, 409, { error: "bill_inactive", message: "the bill is inactive" }).then(() => true);
      const amount: string | null = body.amount ?? bill.amount;
      if (!amount) {
        await json(route, 400, { error: "invalid_bill", message: "invalid bill input: amount is required: the bill has no fixed amount" });
        return true;
      }
      const date: string = body.date || deps.today();
      const category: string = body.category || bill.category;
      const expense = deps.recordExpense({ date, description: body.description || bill.name, category, currency: bill.currency, amount });
      const paid: MockOccurrence = {
        ...bill.pending,
        status: "paid",
        paid_on: date,
        expense_movement_id: expense.id,
        amount_paid: amount,
        currency: bill.currency,
        resolved_at: `${date}T12:00:00Z`,
      };
      bill.history.unshift(paid);
      bill.pending = newPending(nextDue(bill.recurrence, bill.anchor_day, paid.due_date));
      await json(route, 200, {
        bill: dto(bill),
        paid_occurrence: paid,
        expense: { id: expense.id, date, description: body.description || bill.name, category, currency: bill.currency, amount, amount_mxn: expense.amount_mxn },
      });
      return true;
    }

    if (m[2] === "/skip" && method === "POST") {
      writes.push({ method, path: pathname, body: undefined });
      if (fail === "skip") return json(route, 500, { error: "internal_error" }).then(() => true);
      if (!bill) return json(route, 404, { error: "not_found" }).then(() => true);
      if (!bill.active) return json(route, 409, { error: "bill_inactive", message: "the bill is inactive" }).then(() => true);
      bill.history.unshift({ ...bill.pending, status: "skipped", resolved_at: `${deps.today()}T12:00:00Z` });
      bill.pending = newPending(nextDue(bill.recurrence, bill.anchor_day, bill.pending.due_date));
      await json(route, 200, dto(bill));
      return true;
    }
    return false;
  }

  return { bills, writes, handle };
}
