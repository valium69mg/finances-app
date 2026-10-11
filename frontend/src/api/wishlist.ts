import { api } from "./client";
import { personalKeys } from "./personal";

/**
 * Wishlist API (household role): "Lista de deseos". An item has an estimated price, one of her Gasto categories,
 * an optional store link, a priority and a note. The status is derived by the server from what the item became:
 * `convertida` while it links a future expense, a Gasto or a request that is solicitada or aprobada, `activa`
 * otherwise (so a rejected or cancelled request, or a deleted target, brings it back). Amounts travel as decimal strings.
 */

export type WishPriority = "alta" | "media" | "baja";

export const WISH_PRIORITIES: readonly WishPriority[] = ["alta", "media", "baja"];

export type WishStatus = "activa" | "convertida";

/** What the item became: her future expense, her Gasto or an expense request to the owner. */
export type ConversionType = "gasto_futuro" | "gasto" | "peticion";

export type RequestState = "solicitada" | "aprobada" | "rechazada" | "cancelada";

export interface WishConversion {
  type: ConversionType;
  target_id: number;
  /** Only for a `peticion`: its current state. */
  request_state: RequestState | null;
}

export interface WishItem {
  id: number;
  name: string;
  /** Estimated price, decimal string (MXN). */
  price: string;
  category_id: number;
  category: string;
  store_url: string | null;
  priority: WishPriority;
  note: string | null;
  status: WishStatus;
  /**
   * Null for an item that never converted. A rejected or cancelled request keeps reporting itself here (with its
   * state) while the status is back to `activa`, until the item is converted again.
   */
  conversion: WishConversion | null;
  created_at: string;
  updated_at: string;
}

/** Body of create and update. The status and the links are never sent. */
export interface WishInput {
  name: string;
  price: string;
  category_id: number;
  store_url?: string;
  priority: WishPriority;
  note?: string;
}

/** Convert into her future expense: the target defaults to the price. */
export interface ConvertFutureInput {
  target_amount?: string;
  due_date: string;
}

/** Convert into her Gasto ("Ya lo compré"): amount defaults to the price, date to today, category to the item's. */
export interface ConvertExpenseInput {
  amount?: string;
  date?: string;
  category_id?: number;
}

/** Convert into an expense request: `category` is an OWNER Gasto category name (required). */
export interface ConvertRequestInput {
  amount?: string;
  date?: string;
  category: string;
}

export const wishlistKeys = {
  all: [...personalKeys.all, "wishlist"] as const,
  /** The owner's read-only view of one household user's list. */
  of: (userId: string) => [...personalKeys.all, "wishlist", userId] as const,
};

type Client = Pick<typeof api, "request">;

export function createWishlistApi(client: Client = api) {
  return {
    /** Active items first (priority alta to baja, newest first), then the converted ones. */
    list: (userId?: string) => client.request<WishItem[]>(userId ? `/personal/wishlist?user_id=${encodeURIComponent(userId)}` : "/personal/wishlist"),
    create: (input: WishInput) => client.request<WishItem>("/personal/wishlist", { method: "POST", body: input }),
    update: (id: number, input: WishInput) => client.request<WishItem>(`/personal/wishlist/${id}`, { method: "PUT", body: input }),
    /** Only the item goes: what it became is never touched. */
    remove: (id: number) => client.request<void>(`/personal/wishlist/${id}`, { method: "DELETE" }),
    convertToFuture: (id: number, input: ConvertFutureInput) =>
      client.request<WishItem>(`/personal/wishlist/${id}/convert/future-expense`, { method: "POST", body: input }),
    convertToExpense: (id: number, input: ConvertExpenseInput) =>
      client.request<WishItem>(`/personal/wishlist/${id}/convert/expense`, { method: "POST", body: input }),
    convertToRequest: (id: number, input: ConvertRequestInput) =>
      client.request<WishItem>(`/personal/wishlist/${id}/convert/request`, { method: "POST", body: input }),
  };
}

const defaultApi = createWishlistApi();

export const listWishlist = defaultApi.list;
export const createWishItem = defaultApi.create;
export const updateWishItem = defaultApi.update;
export const deleteWishItem = defaultApi.remove;
export const convertWishToFuture = defaultApi.convertToFuture;
export const convertWishToExpense = defaultApi.convertToExpense;
export const convertWishToRequest = defaultApi.convertToRequest;
