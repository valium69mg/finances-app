/**
 * Personal budget cycles (pay cycles), mirroring backend/internal/ledger/domain/cycle.go.
 *
 * A cycle keeps the YYYY-MM label of the month in which it ENDS. `startDay`:
 *  - 0: the cycle is the calendar month (default).
 *  - 1..31: cycle M starts on day `startDay` of month M-1 (clamped to that month's
 *    length, so 31 means its last day) and ends the day before cycle M+1 starts.
 *
 * Only personal views use it; fiscal periods stay on calendar months.
 */

const MONTH_LABEL = /^(\d{4})-(0[1-9]|1[0-2])$/;
const DATE = /^(\d{4})-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/;

const SHORT_MONTHS = ["ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"];

const p2 = (n: number) => String(n).padStart(2, "0");
const isoOf = (t: Date) => `${t.getUTCFullYear()}-${p2(t.getUTCMonth() + 1)}-${p2(t.getUTCDate())}`;
const labelOf = (t: Date) => `${t.getUTCFullYear()}-${p2(t.getUTCMonth() + 1)}`;

/** Day of (year, month0) on which a cycle starts: startDay clamped to the month length. */
function startIn(year: number, month0: number, startDay: number): Date {
  const last = new Date(Date.UTC(year, month0 + 1, 0)).getUTCDate();
  return new Date(Date.UTC(year, month0, Math.min(startDay, last)));
}

/** Label of the cycle a YYYY-MM-DD date belongs to. A date that does not parse falls back to its YYYY-MM prefix. */
export function cycleOf(date: string, startDay: number): string {
  const m = DATE.exec(date);
  if (startDay === 0 || !m) return date.slice(0, 7);
  const [year, month0, day] = [Number(m[1]), Number(m[2]) - 1, Number(m[3])];
  // Cycle M+1 starts inside month M, so a date on or after that day already belongs to the next label.
  if (day >= startIn(year, month0, startDay).getUTCDate()) return labelOf(new Date(Date.UTC(year, month0 + 1, 1)));
  return labelOf(new Date(Date.UTC(year, month0, 1)));
}

/** First and last day (inclusive, YYYY-MM-DD) of the cycle with the given YYYY-MM label, or null if the label is invalid. */
export function cycleRange(label: string, startDay: number): { from: string; to: string } | null {
  const m = MONTH_LABEL.exec(label);
  if (!m) return null;
  const [year, month0] = [Number(m[1]), Number(m[2]) - 1];
  if (startDay === 0) return { from: isoOf(new Date(Date.UTC(year, month0, 1))), to: isoOf(new Date(Date.UTC(year, month0 + 1, 0))) };
  const start = startIn(year, month0 - 1, startDay);
  const next = startIn(year, month0, startDay);
  return { from: isoOf(start), to: isoOf(new Date(next.getTime() - 86_400_000)) };
}

/** Label of the cycle containing `today` (a YYYY-MM-DD date). */
export function currentCycle(today: string, startDay: number): string {
  return cycleOf(today, startDay);
}

/** Label of the cycle before the given one (January rolls to the previous December). */
export function previousCycle(label: string): string {
  const m = MONTH_LABEL.exec(label);
  if (!m) return label;
  return labelOf(new Date(Date.UTC(Number(m[1]), Number(m[2]) - 2, 1)));
}

/** "2026-09-30" -> "30 sep". */
function shortDate(iso: string): string {
  const m = DATE.exec(iso);
  return m ? `${Number(m[3])} ${SHORT_MONTHS[Number(m[2]) - 1]}` : iso;
}

/** Display text of a YYYY-MM-DD range, such as "30 sep – 30 oct". */
export function rangeText(from: string, to: string): string {
  return `${shortDate(from)} – ${shortDate(to)}`;
}

/** Display text of a cycle's range, or an empty string when the label is invalid. */
export function cycleRangeLabel(label: string, startDay: number): string {
  const r = cycleRange(label, startDay);
  return r ? rangeText(r.from, r.to) : "";
}
