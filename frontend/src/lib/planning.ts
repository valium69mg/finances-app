/**
 * Decimal-safe planning helpers for the Configuración tab. Everything works on
 * decimal strings with BigInt integer math, never floats, so 0.165 shows as
 * 16.5 and saves back as 0.165.
 *
 * Base monthly income = salary_usd * fx_rate_applied, rounded to cents. It is an
 * estimate for planning, not the real deposit. When either value is missing or
 * zero the base is "not available" (null) and every percentage feature falls
 * back to the amount-only behavior.
 */

const PLAIN = /^(\d+)(?:\.(\d+))?$/;

interface Dec {
  /** Unscaled digits: the value is n / 10^s. */
  n: bigint;
  s: number;
}

function parse(v: string): Dec | null {
  const m = PLAIN.exec(v.trim());
  if (!m) return null;
  const frac = m[2] ?? "";
  return { n: BigInt(m[1] + frac), s: frac.length };
}

/** Plain non-negative decimal such as "12", "16.5" (no sign, no comma, no exponent). */
export const isPlainDecimal = (v: string) => PLAIN.test(v.trim());

/** Writes n / 10^s as a plain decimal, dropping trailing fraction zeros ("100.50" -> "100.5"). */
function format({ n, s }: Dec): string {
  const digits = n.toString().padStart(s + 1, "0");
  const int = digits.slice(0, digits.length - s);
  const frac = digits.slice(digits.length - s).replace(/0+$/, "");
  return frac ? `${int}.${frac}` : int;
}

/** Integer division rounding half up (both operands non-negative). */
const roundDiv = (a: bigint, b: bigint): bigint => (2n * a + b) / (2n * b);

const pow10 = (e: number): bigint => 10n ** BigInt(e);

/** Value in cents, rounded half up. */
function toCents({ n, s }: Dec): bigint {
  return s <= 2 ? n * pow10(2 - s) : roundDiv(n, pow10(s - 2));
}

const centsToString = (c: bigint): string => {
  const d = c.toString().padStart(3, "0");
  return `${d.slice(0, -2)}.${d.slice(-2)}`;
};

/** Cents of a decimal string, or null when it is not a plain decimal. */
function cents(v: string): bigint | null {
  const d = parse(v);
  return d ? toCents(d) : null;
}

/** "0.165" -> "16.5", "1.0" -> "100", "0.5" -> "50". Returns the input untouched when it is not a plain decimal. */
export function rateToPercent(rate: string): string {
  const d = parse(rate);
  return d ? format({ n: d.n * 100n, s: d.s }) : rate;
}

/** "16.5" -> "0.165", "100" -> "1", "50" -> "0.5". Returns the input untouched when it is not a plain decimal. */
export function percentToRate(percent: string): string {
  const d = parse(percent);
  return d ? format({ n: d.n, s: d.s + 2 }) : percent;
}

/** Exact sum of plain decimals as a plain decimal; a value that is not a plain decimal counts as 0. */
export function sumDecimals(values: string[]): string {
  const parsed = values.map((v) => parse(v) ?? { n: 0n, s: 0 });
  const scale = parsed.reduce((m, d) => Math.max(m, d.s), 0);
  const total = parsed.reduce((acc, d) => acc + d.n * pow10(scale - d.s), 0n);
  return format({ n: total, s: scale });
}

/** True when a plain decimal is exactly 100. */
export function isHundred(percent: string): boolean {
  const d = parse(percent);
  return d !== null && d.n === 100n * pow10(d.s);
}

/** True when a plain decimal is at most 100. */
export function isAtMostHundred(percent: string): boolean {
  const d = parse(percent);
  return d !== null && d.n <= 100n * pow10(d.s);
}

/** salary_usd * fx_rate_applied rounded to cents ("62090.00"), or null when either value is missing, invalid or zero. */
export function baseIncome(salaryUsd: string, fxRate: string): string | null {
  const a = parse(salaryUsd);
  const b = parse(fxRate);
  if (!a || !b || a.n === 0n || b.n === 0n) return null;
  const c = toCents({ n: a.n * b.n, s: a.s + b.s });
  return c > 0n ? centsToString(c) : null;
}

/** Tenths of a percent as text with one decimal ("58" -> "5.8"); a leading minus is kept. */
function tenthsToString(t: bigint): string {
  const abs = t < 0n ? -t : t;
  const s = abs.toString().padStart(2, "0");
  return `${t < 0n ? "-" : ""}${s.slice(0, -1)}.${s.slice(-1)}`;
}

/** Share of an amount in the base with one decimal, half up ("3600" of "62090" -> "5.8"). Null when the amount is not a decimal. */
export function amountToPercent(amount: string, base: string): string | null {
  const a = cents(amount);
  const b = cents(base);
  if (a === null || b === null || b === 0n) return null;
  return tenthsToString(roundDiv(a * 1000n, b));
}

/** Amount for a percentage of the base: base * pct / 100 rounded to cents ("10" of "62090" -> "6209.00"). Null when the percentage is not a decimal. */
export function percentToAmount(percent: string, base: string): string | null {
  const p = parse(percent);
  const b = cents(base);
  if (!p || b === null) return null;
  return centsToString(roundDiv(b * p.n, 100n * pow10(p.s)));
}

/** Money without the currency sign and without zero cents: "3500.00" -> "3,500", "17.50" -> "17.50". */
export function formatPlain(value: string): string {
  const d = parse(value);
  if (!d) return value;
  const [int, frac = ""] = format(d).split(".");
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return frac ? `${grouped}.${frac.padEnd(2, "0")}` : grouped;
}

export interface BudgetLine {
  kind: string;
  /** Raw budget text; empty or invalid counts as no budget. */
  budget: string;
}

export interface Distribution {
  base: string;
  /** Budgets of Gasto categories, of Ahorro categories and both together (two decimals). */
  expense: string;
  saving: string;
  assigned: string;
  /** base - assigned, negative when over. */
  remaining: string;
  /** Percentages of the base with one decimal. */
  assignedPct: string;
  remainingPct: string;
  over: boolean;
  /** Absolute excess when over, else "0.00". */
  excess: string;
  /** Segment widths for the bar, 0-100, for display only. */
  expenseWidth: number;
  savingWidth: number;
}

export const BUDGET_KINDS = ["Gasto", "Ahorro"] as const;
export const isBudgetKind = (kind: string): boolean => (BUDGET_KINDS as readonly string[]).includes(kind);

/**
 * How much of the base the normal budgets of the Gasto and Ahorro categories
 * take. Month overrides and the investment pause plan are not part of it.
 */
export function distribution(lines: BudgetLine[], base: string): Distribution | null {
  const b = cents(base);
  if (b === null || b <= 0n) return null;
  let expense = 0n;
  let saving = 0n;
  for (const l of lines) {
    const c = cents(l.budget);
    if (c === null) continue;
    if (l.kind === "Gasto") expense += c;
    else if (l.kind === "Ahorro") saving += c;
  }
  const assigned = expense + saving;
  const remaining = b - assigned;
  const pct = (c: bigint) => tenthsToString(c < 0n ? -roundDiv(-c * 1000n, b) : roundDiv(c * 1000n, b));
  const denominator = assigned > b ? assigned : b;
  const width = (c: bigint) => Number((c * 10000n) / denominator) / 100;
  return {
    base: centsToString(b),
    expense: centsToString(expense),
    saving: centsToString(saving),
    assigned: centsToString(assigned),
    remaining: `${remaining < 0n ? "-" : ""}${centsToString(remaining < 0n ? -remaining : remaining)}`,
    assignedPct: pct(assigned),
    remainingPct: pct(remaining),
    over: assigned > b,
    excess: assigned > b ? centsToString(assigned - b) : "0.00",
    expenseWidth: width(expense),
    savingWidth: width(saving),
  };
}
