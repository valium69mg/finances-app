/**
 * URL conventions for the Panel's interactive links (phase 12 step 6, reused by the owner's Panel in phase 13):
 * the month travels as `?mes=YYYY-MM` (the pay cycle label) and a category filter as `?categoria=<id>`. Target pages
 * read them as their initial state and keep them in the URL; anything malformed is ignored, never an error.
 */

export const MONTH_PARAM = "mes";
export const CATEGORY_PARAM = "categoria";

const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;

/** A valid YYYY-MM label, or "" for a missing or malformed value. */
export function parseMonthParam(raw: string | null | undefined): string {
  return raw && MONTH.test(raw) ? raw : "";
}

/** A positive integer id, or 0 for a missing or malformed value. */
export function parseCategoryParam(raw: string | null | undefined): number {
  if (!raw || !/^[1-9]\d{0,9}$/.test(raw)) return 0;
  return Number(raw);
}

/** `path?mes=2026-10&categoria=5`; empty or zero values are left out. */
export function linkWithMonth(path: string, month: string, extra: { categoria?: number } = {}): string {
  const params = new URLSearchParams();
  if (month) params.set(MONTH_PARAM, month);
  if (extra.categoria) params.set(CATEGORY_PARAM, String(extra.categoria));
  const query = params.toString();
  return query ? `${path}?${query}` : path;
}
