import type { CloseAdjustment, MonthClose } from "../../api/monthClose";
import { formatMoney, formatPercent } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

/**
 * "2026-10-02T15:04:05Z" -> "2 de octubre de 2026", in the browser's time zone so
 * a close generated late in the evening shows the day the user lived it.
 * Anything that is not a valid instant is returned untouched.
 */
export function closedAtLabel(closedAt: string): string {
  const d = new Date(closedAt);
  if (Number.isNaN(d.getTime())) return closedAt;
  const p = (n: number) => String(n).padStart(2, "0");
  return dateLabel(`${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`);
}

/** Categories whose spending went past a budget, in the order of the close. */
export const overBudgetOf = (close: MonthClose) => close.categories.filter((c) => c.over_budget);

/** Signed percentage with one explicit plus for a rise: "25.5" -> "+25.50%", "-30" -> "-30.00%". */
export function signedPercent(value: string): string {
  const text = formatPercent(value);
  return text.startsWith("-") || text === value ? text : `+${text}`;
}

/** Spanish sentence of a budget-adjustment hint. */
export function describeAdjustment(a: CloseAdjustment): string {
  const direction = a.deviation_pct.startsWith("-") ? "por debajo" : "por encima";
  return `Presupuesto ${formatMoney(a.budget)}, real ${formatMoney(a.real)} (${signedPercent(a.deviation_pct)}, ${direction} del presupuesto).`;
}
