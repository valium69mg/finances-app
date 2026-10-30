import type { Category } from "../../api/settings";
import { amountToPercent } from "../../lib/planning";
import { cycleRangeLabel, currentCycle, LAST_WEEKDAY_START_DAY } from "../cycle";
import { KINDS, toCategoryDraft, type CategoryDraft } from "./validation";

/** A saved category with its position in the saved list (the list is saved as a whole, so the position identifies it). */
export interface IndexedCategory {
  category: Category;
  index: number;
}

export interface CategoryGroup {
  kind: string;
  items: IndexedCategory[];
}

/** Categories grouped by kind: Gasto, Ahorro and Ingreso first, then any other kind in first-seen order. Empty groups are left out. */
export function groupCategories(cats: Category[]): CategoryGroup[] {
  const kinds: string[] = [...KINDS];
  for (const c of cats) if (!kinds.includes(c.kind)) kinds.push(c.kind);
  return kinds
    .map((kind) => ({ kind, items: cats.flatMap((category, index) => (category.kind === kind ? [{ category, index }] : [])) }))
    .filter((g) => g.items.length > 0);
}

/** The list with one item replaced. */
export const replaceAt = <T>(list: T[], index: number, item: T): T[] => list.map((x, i) => (i === index ? item : x));

/**
 * The full category list as the form edits it: the saved categories with one changed
 * (`index`) or, when `index` is null, the new one appended at the end.
 */
export function categoryListWith(saved: Category[], item: CategoryDraft, index: number | null): CategoryDraft[] {
  const drafts = saved.map(toCategoryDraft);
  return index === null ? [...drafts, item] : replaceAt(drafts, index, item);
}

/** Share of the base a budget takes: the percentage text (one decimal) and a 0-100 width for its bar. Null without a budget or a base. */
export function budgetShare(budget: string | null, base: string | null): { percent: string; width: number } | null {
  if (!budget || !base) return null;
  const percent = amountToPercent(budget, base);
  if (percent === null) return null;
  return { percent, width: Math.max(0, Math.min(100, Number(percent))) };
}

/** Spanish name of the period start option. */
export function cycleStartLabel(day: number): string {
  if (day === 0) return "Mes calendario";
  if (day === 31) return "Último día del mes";
  if (day === LAST_WEEKDAY_START_DAY) return "Último día hábil (lun–vie)";
  return `Día ${day}`;
}

/** "Periodo actual: 30 oct → 29 nov" for the period that contains `today` (YYYY-MM-DD). */
export function currentPeriodText(startDay: number, today: string): string {
  const range = cycleRangeLabel(currentCycle(today, startDay), startDay);
  return range ? `Periodo actual: ${range.replace(" – ", " → ")}` : "";
}

const MONTH_NAMES = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];

/** "2027-02" -> "febrero de 2027"; anything else is returned untouched. */
export function formatMonthName(month: string): string {
  const m = /^(\d{4})-(0[1-9]|1[0-2])$/.exec(month.trim());
  return m ? `${MONTH_NAMES[Number(m[2]) - 1]} de ${m[1]}` : month;
}
