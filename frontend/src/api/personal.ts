import { api } from "./client";
import type { MovementKind } from "./dashboard";

/**
 * Personal finances API (household role): her own categories and movements, fully separate from the family
 * ledger. Amounts travel as decimal strings. An Ahorro amount is negative for a withdrawal.
 */

export interface PersonalCategory {
  id: number;
  kind: MovementKind;
  name: string;
  /** Fixed categories (Ingreso and Ahorro) cannot be renamed. */
  fixed: boolean;
}

export interface PersonalMovement {
  id: number;
  kind: MovementKind;
  category_id: number;
  category: string;
  /** Decimal string; negative only for an Ahorro withdrawal. */
  amount: string;
  description: string;
  /** YYYY-MM-DD */
  date: string;
  created_at: string;
  updated_at: string;
}

/** Body for create and update. An empty or missing date means today on the server. */
export interface PersonalMovementInput {
  kind: MovementKind;
  category_id: number;
  amount: string;
  description?: string;
  date?: string;
}

export interface PersonalListFilter {
  /** YYYY-MM cycle label; empty or missing lets the server pick the current cycle. */
  month?: string;
  kind?: MovementKind;
  categoryId?: number;
}

export const personalKeys = {
  all: ["personal"] as const,
  categories: (kind: MovementKind) => ["personal", "categories", kind] as const,
  movements: (kind: MovementKind, month: string) => ["personal", "movements", kind, month] as const,
};

type Client = Pick<typeof api, "request">;

export function createPersonalApi(client: Client = api) {
  return {
    categories: (kind: MovementKind) => client.request<PersonalCategory[]>(`/personal/categories?kind=${encodeURIComponent(kind)}`),
    createCategory: (name: string) => client.request<PersonalCategory>("/personal/categories", { method: "POST", body: { name } }),
    renameCategory: (id: number, name: string) => client.request<PersonalCategory>(`/personal/categories/${id}`, { method: "PUT", body: { name } }),
    movements: (filter: PersonalListFilter = {}) => {
      const params = new URLSearchParams();
      if (filter.month) params.set("month", filter.month);
      if (filter.kind) params.set("kind", filter.kind);
      if (filter.categoryId) params.set("category_id", String(filter.categoryId));
      const query = params.toString();
      return client.request<PersonalMovement[]>(`/personal/movements${query ? `?${query}` : ""}`);
    },
    createMovement: (input: PersonalMovementInput) => client.request<PersonalMovement>("/personal/movements", { method: "POST", body: input }),
    updateMovement: (id: number, input: PersonalMovementInput) =>
      client.request<PersonalMovement>(`/personal/movements/${id}`, { method: "PUT", body: input }),
    removeMovement: (id: number) => client.request<void>(`/personal/movements/${id}`, { method: "DELETE" }),
  };
}

const defaultApi = createPersonalApi();

export const listPersonalCategories = defaultApi.categories;
export const createPersonalCategory = defaultApi.createCategory;
export const renamePersonalCategory = defaultApi.renameCategory;
export const listPersonalMovements = defaultApi.movements;
export const createPersonalMovement = defaultApi.createMovement;
export const updatePersonalMovement = defaultApi.updateMovement;
export const deletePersonalMovement = defaultApi.removeMovement;
