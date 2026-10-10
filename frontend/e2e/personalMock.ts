import type { Request, Route } from "@playwright/test";

/**
 * In-memory stand-in for the /personal endpoints, used by helpers.ts. It keeps what the backend guarantees: household
 * only (the owner gets 403), categories seeded per kind, Gasto categories editable (409 category_exists on a clash,
 * 409 category_fixed for fixed kinds), amount > 0 except Ahorro (any non-zero, negative is a withdrawal), newest first.
 * All examples are fake: the repository is public.
 */

export type PersonalKind = "Ingreso" | "Gasto" | "Ahorro";

/** A future expense of hers; `saved` becomes an Ahorro in Gastos futuros linked to it. */
export interface MockPersonalFuture {
  id: number;
  name: string;
  target_amount: string;
  due_date: string;
  saved?: string;
  status?: "active" | "paid";
  paid_at?: string | null;
  amount_paid?: string | null;
}

export interface MockPersonalMovement {
  id: number;
  kind: PersonalKind;
  /** Category name; the mock resolves it to the seeded category id. */
  category: string;
  amount: string;
  description?: string;
  date: string;
}

/** An item of her wishlist. `request_id` links it to an expense request of the requests mock (seeded by the test). */
export interface MockPersonalWish {
  id: number;
  name: string;
  price: string;
  /** Gasto category name of hers. */
  category: string;
  store_url?: string | null;
  priority?: "alta" | "media" | "baja";
  note?: string | null;
  request_id?: number;
  future_expense_id?: number;
  expense_movement_id?: number;
}

type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

/** The expense requests mock, as the wishlist conversion to a request needs it. */
export interface RequestsPort {
  create: (body: Record<string, unknown>) => { status: number; body: unknown };
  stateOf: (id: number) => string | null;
}

export interface PersonalDeps {
  role: "owner" | "household";
  today: () => string;
  /** Cycle label of a date, as the owner's cut day defines it. */
  cycleOf: (date: string) => string;
  json: Json;
  requests: RequestsPort;
}

interface Category {
  id: number;
  kind: PersonalKind;
  name: string;
}

const SEED: Record<PersonalKind, string[]> = {
  Ingreso: ["Trabajo", "Transferencia", "Otros"],
  Gasto: ["Belleza", "Salud", "Tratamientos", "Tienda de conveniencia", "Ropa", "Pintura", "Maquillaje", "Compras en línea"],
  Ahorro: ["Ahorro", "Inversión", "Gastos futuros"],
};

const validAmount = (v: unknown) => typeof v === "string" && /^-?\d{1,12}(\.\d{1,2})?$/.test(v) && Number(v) !== 0;

export function createPersonalMock(seed: MockPersonalMovement[], deps: PersonalDeps, futureSeed: MockPersonalFuture[] = [], wishSeed: MockPersonalWish[] = []) {
  const categories: Category[] = [];
  let nextCategoryId = 1;
  (Object.keys(SEED) as PersonalKind[]).forEach((kind) => SEED[kind].forEach((name) => categories.push({ id: nextCategoryId++, kind, name })));
  const byName = (kind: PersonalKind, name: string) => categories.find((c) => c.kind === kind && c.name === name);

  interface Stored {
    id: number;
    kind: PersonalKind;
    category_id: number;
    amount: string;
    description: string;
    date: string;
    future_expense_id: number | null;
  }
  let nextId = seed.reduce((max, m) => Math.max(max, m.id), 0) + 1;
  const movements: Stored[] = seed.map((m) => ({
    id: m.id,
    kind: m.kind,
    category_id: byName(m.kind, m.category)?.id ?? 0,
    amount: m.amount,
    description: m.description ?? "",
    date: m.date,
    future_expense_id: null,
  }));
  /** Every write the page sent: method, path and JSON body. */
  const writes: { method: string; path: string; body: unknown }[] = [];
  /** Months asked of GET /personal/movements, in order (empty string when none was sent). */
  const months: string[] = [];

  const categoryDto = (c: Category) => ({ id: c.id, kind: c.kind, name: c.name, fixed: c.kind !== "Gasto" });
  const movementDto = (m: Stored) => ({
    id: m.id,
    kind: m.kind,
    category_id: m.category_id,
    category: categories.find((c) => c.id === m.category_id)?.name ?? "",
    amount: m.amount,
    description: m.description,
    date: m.date,
    future_expense_id: m.future_expense_id,
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
  });

  function validate(body: Record<string, unknown>): string | null {
    const kind = body.kind as PersonalKind;
    if (!SEED[kind]) return "kind is invalid";
    if (!validAmount(body.amount)) return "amount is invalid";
    if (kind !== "Ahorro" && Number(body.amount) < 0) return "amount must be positive";
    const cat = categories.find((c) => c.id === body.category_id);
    if (!cat || cat.kind !== kind) return "category is invalid";
    return null;
  }


  // --- future expenses ---------------------------------------------------

  /** Every write the future expenses page sent: method, path and JSON body. */
  const futureWrites: { method: string; path: string; body: unknown }[] = [];

  interface StoredFuture {
    id: number;
    name: string;
    targetCents: number;
    due_date: string;
    status: "active" | "paid";
    paid_at: string | null;
    amountPaidCents: number | null;
    expense_movement_id: number | null;
  }
  const cents = (v: unknown) => Math.round(Number(v) * 100);
  const money = (c: number) => (c / 100).toFixed(2);
  const validPositive = (v: unknown) => typeof v === "string" && /^\d{1,12}(\.\d{1,2})?$/.test(v) && cents(v) > 0;
  const monthIndex = (label: string) => Number(label.slice(0, 4)) * 12 + Number(label.slice(5, 7));
  const futureCategory = () => byName("Ahorro", "Gastos futuros")?.id ?? 0;
  let nextFutureId = futureSeed.reduce((max, f) => Math.max(max, f.id), 0) + 1;
  const futures: StoredFuture[] = futureSeed.map((f) => ({
    id: f.id,
    name: f.name,
    targetCents: cents(f.target_amount),
    due_date: f.due_date,
    status: f.status ?? "active",
    paid_at: f.paid_at ?? null,
    amountPaidCents: f.amount_paid ? cents(f.amount_paid) : null,
    expense_movement_id: null,
  }));
  futureSeed.forEach((f) => {
    if (f.saved && cents(f.saved) !== 0 && (f.status ?? "active") === "active") {
      movements.push({ id: nextId++, kind: "Ahorro", category_id: futureCategory(), amount: f.saved, description: `Ahorro para ${f.name}`, date: deps.today(), future_expense_id: f.id });
    }
  });
  const savedCents = (id: number) => movements.filter((m) => m.future_expense_id === id).reduce((t, m) => t + cents(m.amount), 0);
  const freeCents = () =>
    movements.filter((m) => m.kind === "Ahorro" && m.category_id === futureCategory() && m.future_expense_id === null).reduce((t, m) => t + cents(m.amount), 0);

  const futureDto = (f: StoredFuture) => {
    const paid = f.status === "paid";
    const cycles = Math.max(1, monthIndex(deps.cycleOf(f.due_date)) - monthIndex(deps.cycleOf(deps.today())));
    const saved = savedCents(f.id);
    const remaining = paid ? 0 : Math.max(0, f.targetCents - saved);
    return {
      id: f.id,
      name: f.name,
      target_amount: money(f.targetCents),
      due_date: f.due_date,
      status: f.status,
      saved: money(paid ? 0 : saved),
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

  function validateFuture(body: Record<string, unknown>): string | null {
    if (typeof body.name !== "string" || body.name.trim() === "") return "invalid future expense: name is required";
    if (!validPositive(body.target_amount)) return "invalid future expense: target amount must be greater than zero";
    if (typeof body.due_date !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(body.due_date)) return "invalid future expense: due date must be a date like YYYY-MM-DD";
    return null;
  }

  async function handleFuture(route: Route, request: Request, pathname: string): Promise<boolean> {
    const { json } = deps;
    const method = request.method();
    const reply = (status: number, body?: unknown) => json(route, status, body).then(() => true);
    const alreadyPaid = () => reply(409, { error: "already_paid", message: "future expense is already paid" });

    if (pathname === "/personal/future-expenses" && method === "GET") {
      const activeItems = futures.filter((f) => f.status === "active").sort((a, b) => (a.due_date === b.due_date ? a.id - b.id : a.due_date < b.due_date ? -1 : 1));
      const paidItems = futures.filter((f) => f.status === "paid").sort((a, b) => ((b.paid_at ?? "") < (a.paid_at ?? "") ? -1 : 1));
      const list = activeItems.map(futureDto);
      const sum = (key: "target_amount" | "saved" | "remaining" | "suggested_monthly") => money(list.reduce((t, i) => t + cents(i[key]), 0));
      return reply(200, {
        active: list,
        paid: paidItems.map(futureDto),
        totals: { target: sum("target_amount"), saved: sum("saved"), remaining: sum("remaining"), suggested_monthly: sum("suggested_monthly") },
        free_balance: money(freeCents()),
      });
    }

    if (pathname === "/personal/future-expenses" && method === "POST") {
      const body = request.postDataJSON();
      futureWrites.push({ method, path: pathname, body });
      const problem = validateFuture(body);
      if (problem) return reply(400, { error: "invalid_future_expense", message: problem });
      const item: StoredFuture = {
        id: nextFutureId++,
        name: body.name.trim(),
        targetCents: cents(body.target_amount),
        due_date: body.due_date,
        status: "active",
        paid_at: null,
        amountPaidCents: null,
        expense_movement_id: null,
      };
      futures.push(item);
      return reply(201, futureDto(item));
    }

    const m = /^\/personal\/future-expenses\/(\d+)(\/savings|\/assign|\/pay)?$/.exec(pathname);
    if (!m) return false;
    const item = futures.find((f) => f.id === Number(m[1]));
    const action = m[2];

    if (!action && method === "GET") return item ? reply(200, futureDto(item)) : reply(404, { error: "not_found" });

    if (!action && method === "PUT") {
      const body = request.postDataJSON();
      futureWrites.push({ method, path: pathname, body });
      if (!item) return reply(404, { error: "not_found" });
      if (item.status === "paid") return alreadyPaid();
      const problem = validateFuture(body);
      if (problem) return reply(400, { error: "invalid_future_expense", message: problem });
      item.name = body.name.trim();
      item.targetCents = cents(body.target_amount);
      item.due_date = body.due_date;
      return reply(200, futureDto(item));
    }

    if (!action && method === "DELETE") {
      futureWrites.push({ method, path: pathname, body: undefined });
      if (!item) return reply(404, { error: "not_found" });
      // The linked savings are unlinked, never deleted: they become free balance.
      movements.forEach((mv) => {
        if (mv.future_expense_id === item.id) mv.future_expense_id = null;
      });
      futures.splice(futures.indexOf(item), 1);
      return reply(204);
    }

    if (method !== "POST" || !action) return false;
    const body = request.postDataJSON() ?? {};
    futureWrites.push({ method, path: pathname, body });
    if (!item) return reply(404, { error: "not_found" });
    if (item.status === "paid") return alreadyPaid();
    const date = body.date || deps.today();

    if (action === "/savings" || action === "/assign") {
      if (!validPositive(body.amount)) return reply(400, { error: "invalid_future_expense", message: "invalid future expense: amount must be greater than zero" });
      const amount = cents(body.amount);
      if (action === "/assign") {
        if (amount > freeCents()) return reply(409, { error: "insufficient_free_balance", message: "not enough free balance" });
        movements.push({ id: nextId++, kind: "Ahorro", category_id: futureCategory(), amount: money(-amount), description: `Asignado a ${item.name}`, date, future_expense_id: null });
        movements.push({ id: nextId++, kind: "Ahorro", category_id: futureCategory(), amount: money(amount), description: `Asignado a ${item.name}`, date, future_expense_id: item.id });
        return reply(200, futureDto(item));
      }
      const saving: Stored = { id: nextId++, kind: "Ahorro", category_id: futureCategory(), amount: money(amount), description: `Ahorro para ${item.name}`, date, future_expense_id: item.id };
      movements.push(saving);
      return reply(201, { item: futureDto(item), saving: movementDto(saving) });
    }

    // /pay: her Gasto in a category she picks (required), the release row and the remainder.
    if (body.amount !== undefined && !validPositive(body.amount)) return reply(400, { error: "invalid_future_expense", message: "invalid future expense: amount must be greater than zero" });
    const category = categories.find((c) => c.id === body.category_id);
    if (!category || category.kind !== "Gasto") return reply(400, { error: "invalid_personal", message: "invalid personal finance input: category_id does not exist" });
    const paidCents = body.amount === undefined ? item.targetCents : cents(body.amount);
    const saved = savedCents(item.id);
    const expense: Stored = { id: nextId++, kind: "Gasto", category_id: category.id, amount: money(paidCents), description: item.name, date, future_expense_id: null };
    movements.push(expense);
    if (saved > 0) {
      movements.push({ id: nextId++, kind: "Ahorro", category_id: futureCategory(), amount: money(-saved), description: `Pago de ${item.name}`, date, future_expense_id: item.id });
      if (saved > paidCents) {
        movements.push({ id: nextId++, kind: "Ahorro", category_id: futureCategory(), amount: money(saved - paidCents), description: `Sobrante de ${item.name}`, date, future_expense_id: null });
      }
    }
    item.status = "paid";
    item.paid_at = date;
    item.amountPaidCents = paidCents;
    item.expense_movement_id = expense.id;
    return reply(200, { item: futureDto(item), expense: movementDto(expense) });
  }


  // --- wishlist ----------------------------------------------------------

  /** Every write the wishlist page sent: method, path and JSON body. */
  const wishWrites: { method: string; path: string; body: unknown }[] = [];

  interface StoredWish {
    id: number;
    name: string;
    priceCents: number;
    category_id: number;
    store_url: string | null;
    priority: "alta" | "media" | "baja";
    note: string | null;
    future_expense_id: number | null;
    expense_movement_id: number | null;
    expense_request_id: number | null;
  }
  let nextWishId = wishSeed.reduce((max, w) => Math.max(max, w.id), 0) + 1;
  const wishes: StoredWish[] = wishSeed.map((w) => ({
    id: w.id,
    name: w.name,
    priceCents: cents(w.price),
    category_id: byName("Gasto", w.category)?.id ?? 0,
    store_url: w.store_url ?? null,
    priority: w.priority ?? "media",
    note: w.note ?? null,
    future_expense_id: w.future_expense_id ?? null,
    expense_movement_id: w.expense_movement_id ?? null,
    expense_request_id: w.request_id ?? null,
  }));
  const RANK = { alta: 0, media: 1, baja: 2 };

  /** What the item links to. A target that no longer exists counts as no link, like the database's ON DELETE SET NULL. */
  function conversionOf(w: StoredWish) {
    if (w.future_expense_id !== null && futures.some((f) => f.id === w.future_expense_id)) return { type: "gasto_futuro", target_id: w.future_expense_id, request_state: null };
    if (w.expense_movement_id !== null && movements.some((m) => m.id === w.expense_movement_id)) return { type: "gasto", target_id: w.expense_movement_id, request_state: null };
    if (w.expense_request_id !== null) {
      const state = deps.requests.stateOf(w.expense_request_id);
      if (state !== null) return { type: "peticion", target_id: w.expense_request_id, request_state: state };
    }
    return null;
  }
  const statusOf = (w: StoredWish) => {
    const c = conversionOf(w);
    if (!c) return "activa";
    return c.type === "peticion" && c.request_state !== "solicitada" && c.request_state !== "aprobada" ? "activa" : "convertida";
  };
  const wishDto = (w: StoredWish) => ({
    id: w.id,
    name: w.name,
    price: money(w.priceCents),
    category_id: w.category_id,
    category: categories.find((c) => c.id === w.category_id)?.name ?? "",
    store_url: w.store_url,
    priority: w.priority,
    note: w.note,
    status: statusOf(w),
    conversion: conversionOf(w),
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
  });

  function validateWish(body: Record<string, unknown>): string | null {
    if (typeof body.name !== "string" || body.name.trim() === "") return "invalid wishlist item: name is required";
    if (!validPositive(body.price)) return "invalid wishlist item: price must be greater than zero";
    const cat = categories.find((c) => c.id === body.category_id);
    if (!cat || cat.kind !== "Gasto") return "invalid wishlist item: category_id is not one of your Gasto categories";
    if (body.store_url !== undefined && !(typeof body.store_url === "string" && /^https?:\/\/\S+$/i.test(body.store_url))) return "invalid wishlist item: store_url must be a valid http or https link";
    if (body.priority !== undefined && !(typeof body.priority === "string" && body.priority in RANK)) return "invalid wishlist item: priority must be alta, media or baja";
    return null;
  }

  async function handleWishlist(route: Route, request: Request, pathname: string): Promise<boolean> {
    const { json } = deps;
    const method = request.method();
    const reply = (status: number, body?: unknown) => json(route, status, body).then(() => true);

    if (pathname === "/personal/wishlist" && method === "GET") {
      const active = wishes.filter((w) => statusOf(w) === "activa").sort((a, b) => RANK[a.priority] - RANK[b.priority] || b.id - a.id);
      const converted = wishes.filter((w) => statusOf(w) === "convertida").sort((a, b) => b.id - a.id);
      return reply(200, [...active, ...converted].map(wishDto));
    }

    if (pathname === "/personal/wishlist" && method === "POST") {
      const body = request.postDataJSON();
      wishWrites.push({ method, path: pathname, body });
      const problem = validateWish(body);
      if (problem) return reply(400, { error: "invalid_wishlist_item", message: problem });
      const item: StoredWish = {
        id: nextWishId++,
        name: body.name.trim(),
        priceCents: cents(body.price),
        category_id: body.category_id,
        store_url: body.store_url ?? null,
        priority: body.priority ?? "media",
        note: body.note ?? null,
        future_expense_id: null,
        expense_movement_id: null,
        expense_request_id: null,
      };
      wishes.push(item);
      return reply(201, wishDto(item));
    }

    const m = /^\/personal\/wishlist\/(\d+)(\/convert\/(future-expense|expense|request))?$/.exec(pathname);
    if (!m) return false;
    const item = wishes.find((w) => w.id === Number(m[1]));
    const way = m[3];

    if (!way && method === "PUT") {
      const body = request.postDataJSON();
      wishWrites.push({ method, path: pathname, body });
      if (!item) return reply(404, { error: "not_found" });
      const problem = validateWish(body);
      if (problem) return reply(400, { error: "invalid_wishlist_item", message: problem });
      item.name = body.name.trim();
      item.priceCents = cents(body.price);
      item.category_id = body.category_id;
      item.store_url = body.store_url ?? null;
      item.priority = body.priority ?? "media";
      item.note = body.note ?? null;
      return reply(200, wishDto(item));
    }

    if (!way && method === "DELETE") {
      wishWrites.push({ method, path: pathname, body: undefined });
      if (!item) return reply(404, { error: "not_found" });
      wishes.splice(wishes.indexOf(item), 1);
      return reply(204);
    }

    if (!way || method !== "POST") return false;
    const body = request.postDataJSON() ?? {};
    wishWrites.push({ method, path: pathname, body });
    if (!item) return reply(404, { error: "not_found" });
    if (statusOf(item) !== "activa") return reply(409, { error: "already_converted" });
    const amount = body.amount === undefined ? item.priceCents : cents(body.amount);
    const date = body.date || deps.today();
    const clearLinks = () => {
      item.future_expense_id = item.expense_movement_id = item.expense_request_id = null;
    };

    if (way === "future-expense") {
      const target = body.target_amount === undefined ? item.priceCents : cents(body.target_amount);
      if (body.target_amount !== undefined && !validPositive(body.target_amount)) return reply(400, { error: "invalid_future_expense", message: "invalid future expense: target amount must be greater than zero" });
      if (typeof body.due_date !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(body.due_date)) return reply(400, { error: "invalid_future_expense", message: "invalid future expense: due date must be a date like YYYY-MM-DD" });
      const created: StoredFuture = { id: nextFutureId++, name: item.name, targetCents: target, due_date: body.due_date, status: "active", paid_at: null, amountPaidCents: null, expense_movement_id: null };
      futures.push(created);
      clearLinks();
      item.future_expense_id = created.id;
      return reply(200, wishDto(item));
    }

    if (way === "expense") {
      if (body.amount !== undefined && !validPositive(body.amount)) return reply(400, { error: "invalid_wishlist_item", message: "invalid wishlist item: amount must be greater than zero" });
      const category = categories.find((c) => c.id === (body.category_id ?? item.category_id));
      if (!category || category.kind !== "Gasto") return reply(400, { error: "invalid_wishlist_item", message: "invalid wishlist item: category_id is not one of your Gasto categories" });
      const expense: Stored = { id: nextId++, kind: "Gasto", category_id: category.id, amount: money(amount), description: item.name, date, future_expense_id: null };
      movements.push(expense);
      clearLinks();
      item.expense_movement_id = expense.id;
      return reply(200, wishDto(item));
    }

    // request: created as the session user through the requests mock (its validation and limits apply).
    if (typeof body.category !== "string" || body.category === "") return reply(400, { error: "invalid_wishlist_item", message: "invalid wishlist item: category is required" });
    const created = deps.requests.create({ amount: money(amount), description: item.name, suggested_category: body.category, date });
    if (created.status !== 201) return reply(created.status, created.body);
    clearLinks();
    item.expense_request_id = (created.body as { id: number }).id;
    return reply(200, wishDto(item));
  }

  async function handle(route: Route, request: Request, pathname: string, searchParams: URLSearchParams): Promise<boolean> {
    if (!pathname.startsWith("/personal/")) return false;
    const { json } = deps;
    if (deps.role !== "household") {
      await json(route, 403, { error: "forbidden" });
      return true;
    }
    const method = request.method();

    if (pathname.startsWith("/personal/future-expenses")) return handleFuture(route, request, pathname);
    if (pathname.startsWith("/personal/wishlist")) return handleWishlist(route, request, pathname);

    if (pathname === "/personal/categories") {
      if (method === "GET") {
        const kind = searchParams.get("kind") as PersonalKind;
        await json(route, 200, categories.filter((c) => c.kind === kind).map(categoryDto));
        return true;
      }
      if (method === "POST") {
        const body = request.postDataJSON();
        writes.push({ method, path: pathname, body });
        const name = String(body.name ?? "").trim();
        if (!name) return json(route, 400, { error: "invalid_personal", message: "name is required" }), true;
        if (categories.some((c) => c.kind === "Gasto" && c.name.toLowerCase() === name.toLowerCase())) {
          await json(route, 409, { error: "category_exists", message: "category name already exists" });
          return true;
        }
        const created = { id: nextCategoryId++, kind: "Gasto" as const, name };
        categories.push(created);
        await json(route, 201, categoryDto(created));
        return true;
      }
    }

    const catMatch = /^\/personal\/categories\/(\d+)$/.exec(pathname);
    if (catMatch && method === "PUT") {
      const body = request.postDataJSON();
      writes.push({ method, path: pathname, body });
      const cat = categories.find((c) => c.id === Number(catMatch[1]));
      if (!cat) return json(route, 404, { error: "not_found" }), true;
      if (cat.kind !== "Gasto") return json(route, 409, { error: "category_fixed", message: "category is fixed" }), true;
      const name = String(body.name ?? "").trim();
      if (categories.some((c) => c.id !== cat.id && c.kind === "Gasto" && c.name.toLowerCase() === name.toLowerCase())) {
        await json(route, 409, { error: "category_exists", message: "category name already exists" });
        return true;
      }
      cat.name = name;
      await json(route, 200, categoryDto(cat));
      return true;
    }

    if (pathname === "/personal/movements") {
      if (method === "GET") {
        const month = searchParams.get("month") ?? "";
        months.push(month);
        const kind = searchParams.get("kind");
        const wanted = month || deps.cycleOf(deps.today());
        const rows = movements
          .filter((m) => (!kind || m.kind === kind) && deps.cycleOf(m.date) === wanted)
          .sort((a, b) => (a.date === b.date ? b.id - a.id : a.date < b.date ? 1 : -1));
        await json(route, 200, rows.map(movementDto));
        return true;
      }
      if (method === "POST") {
        const body = request.postDataJSON();
        writes.push({ method, path: pathname, body });
        const problem = validate(body);
        if (problem) return json(route, 400, { error: "invalid_personal", message: problem }), true;
        const stored: Stored = { id: nextId++, kind: body.kind, category_id: body.category_id, amount: body.amount, description: body.description ?? "", date: body.date || deps.today(), future_expense_id: null };
        movements.push(stored);
        await json(route, 201, movementDto(stored));
        return true;
      }
    }

    const movMatch = /^\/personal\/movements\/(\d+)$/.exec(pathname);
    if (movMatch) {
      const at = movements.findIndex((m) => m.id === Number(movMatch[1]));
      if (method === "PUT") {
        const body = request.postDataJSON();
        writes.push({ method, path: pathname, body });
        if (at < 0) return json(route, 404, { error: "not_found" }), true;
        const problem = validate(body);
        if (problem) return json(route, 400, { error: "invalid_personal", message: problem }), true;
        movements[at] = { ...movements[at], kind: body.kind, category_id: body.category_id, amount: body.amount, description: body.description ?? "", date: body.date || deps.today() };
        await json(route, 200, movementDto(movements[at]));
        return true;
      }
      if (method === "DELETE") {
        writes.push({ method, path: pathname, body: undefined });
        if (at < 0) return json(route, 404, { error: "not_found" }), true;
        movements.splice(at, 1);
        await json(route, 204);
        return true;
      }
    }
    return false;
  }

  return { handle, writes, months, futureWrites, wishWrites };
}
