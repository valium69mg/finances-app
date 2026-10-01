/**
 * Geometry of the budget fit bar. Everything is computed on integer cents (decimal strings are never turned
 * into floats), and the bar is scaled to whichever is larger, the budget or what would be spent, so an excess
 * is always visible past the budget marker.
 */

export interface FitSegments {
  /** Already spent, inside the budget (percent of the bar). */
  spentWithin: number;
  /** The request, inside the budget. */
  requestWithin: number;
  /** Already spent beyond the budget (the category was over before the request). */
  spentOver: number;
  /** The part of the request beyond the budget. */
  requestOver: number;
  /** Position of the budget limit, only when something goes past it. */
  budgetMarker: number | null;
}

function toCents(value: string): bigint | null {
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(value.trim());
  if (!m) return null;
  const cents = BigInt(m[2] + (m[3] ?? "").padEnd(2, "0").slice(0, 2));
  return m[1] ? -cents : cents;
}

const min = (a: bigint, b: bigint) => (a < b ? a : b);
const max = (a: bigint, b: bigint) => (a > b ? a : b);

/** Segments of the bar, or null when a figure is not a plain decimal or the budget is not positive. */
export function fitSegments(budget: string, spent: string, amount: string): FitSegments | null {
  const b = toCents(budget);
  const rawSpent = toCents(spent);
  const a = toCents(amount);
  if (b === null || rawSpent === null || a === null || b <= 0n || a < 0n) return null;
  const s = max(rawSpent, 0n);
  const projected = s + a;
  const scale = max(b, projected);
  const pct = (x: bigint) => Number((x * 10000n) / scale) / 100;

  const spentWithin = min(s, b);
  const requestWithin = min(projected, b) - spentWithin;
  const spentOver = max(s - b, 0n);
  const requestOver = max(projected - b, 0n) - spentOver;
  return {
    spentWithin: pct(spentWithin),
    requestWithin: pct(requestWithin),
    spentOver: pct(spentOver),
    requestOver: pct(requestOver),
    budgetMarker: projected > b ? pct(b) : null,
  };
}
