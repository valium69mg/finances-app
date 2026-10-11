/**
 * URL conventions for the Panel's interactive links (phase 12 step 6, reused by the owner's Panel in phase 13):
 * the month travels as `?mes=YYYY-MM` (the pay cycle label) and a category filter as `?categoria=<id>` (hers) or `?categoria=<name>` (the owner's). Target pages
 * read them as their initial state and keep them in the URL; anything malformed is ignored, never an error.
 */

export const MONTH_PARAM = "mes";
export const CATEGORY_PARAM = "categoria";
/** Deep link to the edit form of one record of the page (movement, future expense): `?editar=<id>`. */
export const EDIT_PARAM = "editar";
/** Deep link to the pay dialog of one bill on Pagos recurrentes: `?pago=<id>`. */
export const PAY_PARAM = "pago";

const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;

/** A valid YYYY-MM label, or "" for a missing or malformed value. */
export function parseMonthParam(raw: string | null | undefined): string {
  return raw && MONTH.test(raw) ? raw : "";
}

/** A positive integer id, or 0 for a missing or malformed value. */
export function parseIdParam(raw: string | null | undefined): number {
  if (!raw || !/^[1-9]\d{0,9}$/.test(raw)) return 0;
  return Number(raw);
}

/** The id of a household category filter (`?categoria=<id>`); same rules as any id. */
export const parseCategoryParam = parseIdParam;

/** The name of an owner category filter (`?categoria=<name>`), or "" for a missing or blank value. */
export function parseCategoryNameParam(raw: string | null | undefined): string {
  const name = raw?.trim() ?? "";
  return name.length > 0 && name.length <= 100 ? name : "";
}

interface LinkExtra {
  /** Her category id, or the owner's category name. */
  categoria?: number | string;
  editar?: number;
  pago?: number;
}

/** `path?mes=2026-10&categoria=5`; empty or zero values are left out. */
export function linkWithMonth(path: string, month: string, extra: LinkExtra = {}): string {
  const params = new URLSearchParams();
  if (month) params.set(MONTH_PARAM, month);
  if (extra.categoria) params.set(CATEGORY_PARAM, String(extra.categoria));
  if (extra.editar) params.set(EDIT_PARAM, String(extra.editar));
  if (extra.pago) params.set(PAY_PARAM, String(extra.pago));
  const query = params.toString();
  return query ? `${path}?${query}` : path;
}
