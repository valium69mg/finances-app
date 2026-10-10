import type { Request, Route } from "@playwright/test";

/**
 * In-memory stand-in for the /personal endpoints, used by helpers.ts. It keeps what the backend guarantees: household
 * only (the owner gets 403), categories seeded per kind, Gasto categories editable (409 category_exists on a clash,
 * 409 category_fixed for fixed kinds), amount > 0 except Ahorro (any non-zero, negative is a withdrawal), newest first.
 * All examples are fake: the repository is public.
 */

export type PersonalKind = "Ingreso" | "Gasto" | "Ahorro";

export interface MockPersonalMovement {
  id: number;
  kind: PersonalKind;
  /** Category name; the mock resolves it to the seeded category id. */
  category: string;
  amount: string;
  description?: string;
  date: string;
}

type Json = (route: Route, status: number, body?: unknown) => Promise<void>;

export interface PersonalDeps {
  role: "owner" | "household";
  today: () => string;
  /** Cycle label of a date, as the owner's cut day defines it. */
  cycleOf: (date: string) => string;
  json: Json;
}

interface Category {
  id: number;
  kind: PersonalKind;
  name: string;
}

const SEED: Record<PersonalKind, string[]> = {
  Ingreso: ["Trabajo", "Transferencia", "Otros"],
  Gasto: ["Belleza", "Salud", "Tratamientos", "Tienda de conveniencia", "Ropa", "Pintura", "Maquillaje", "Compras en línea"],
  Ahorro: ["Ahorro", "Inversión"],
};

const validAmount = (v: unknown) => typeof v === "string" && /^-?\d{1,12}(\.\d{1,2})?$/.test(v) && Number(v) !== 0;

export function createPersonalMock(seed: MockPersonalMovement[], deps: PersonalDeps) {
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
  }
  let nextId = seed.reduce((max, m) => Math.max(max, m.id), 0) + 1;
  const movements: Stored[] = seed.map((m) => ({
    id: m.id,
    kind: m.kind,
    category_id: byName(m.kind, m.category)?.id ?? 0,
    amount: m.amount,
    description: m.description ?? "",
    date: m.date,
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

  async function handle(route: Route, request: Request, pathname: string, searchParams: URLSearchParams): Promise<boolean> {
    if (!pathname.startsWith("/personal/")) return false;
    const { json } = deps;
    if (deps.role !== "household") {
      await json(route, 403, { error: "forbidden" });
      return true;
    }
    const method = request.method();

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
        const stored: Stored = { id: nextId++, kind: body.kind, category_id: body.category_id, amount: body.amount, description: body.description ?? "", date: body.date || deps.today() };
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

  return { handle, writes, months };
}
