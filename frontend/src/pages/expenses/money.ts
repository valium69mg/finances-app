/**
 * Formats a decimal string as money without ever converting it to a float:
 * rounds half away from zero to 2 decimals using integer math and groups thousands.
 * Returns the input untouched when it is not a plain decimal.
 */
export function formatMoney(value: string): string {
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(value.trim());
  if (!m) return value;
  const [, sign, int, frac = ""] = m;
  const cents = BigInt(int + (frac + "00").slice(0, 2)) + (frac.length > 2 && frac[2] >= "5" ? 1n : 0n);
  const s = cents.toString().padStart(3, "0");
  const whole = s.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const isZero = cents === 0n;
  return `${sign && !isZero ? "-" : ""}$${whole}.${s.slice(-2)}`;
}

/** Local calendar date as YYYY-MM-DD. */
export function todayISO(now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`;
}

/** Positive plain decimal such as "12", "12.50" (no commas, no sign). */
export const isPositiveDecimal = (v: string) => /^\d+(\.\d+)?$/.test(v.trim()) && /[1-9]/.test(v);

/**
 * Formats a decimal rate string as a percentage by shifting the decimal point,
 * never through floats: "0.015" -> "1.5%", "0.01" -> "1%", "0.0125" -> "1.25%".
 * Returns the input untouched when it is not a plain decimal.
 */
export function formatRatePercent(rate: string): string {
  const m = /^(\d+)(?:\.(\d+))?$/.exec(rate.trim());
  if (!m) return rate;
  const [, int, frac = ""] = m;
  const all = int + frac.padEnd(2, "0");
  const pointAt = int.length + 2;
  const whole = all.slice(0, pointAt).replace(/^0+(?=\d)/, "");
  const decimals = all.slice(pointAt).replace(/0+$/, "");
  return `${whole}${decimals ? `.${decimals}` : ""}%`;
}

/** Non-zero plain decimal with an optional leading minus, such as "250", "-250.50" (a withdrawal). */
export const isNonZeroDecimal = (v: string) => /^-?\d+(\.\d+)?$/.test(v.trim()) && /[1-9]/.test(v);

/**
 * Formats a percentage that is already expressed as such ("12.5" -> "12.50%")
 * with the same integer rounding as formatMoney. Returns the input untouched
 * when it is not a plain decimal.
 */
export function formatPercent(value: string): string {
  const money = formatMoney(value);
  return money === value ? value : `${money.replace("$", "")}%`;
}

/**
 * Share of `part` in `total` as a number between 0 and 100 with two decimals,
 * computed with integer math. Only meant for display (bar widths, labels).
 */
export function percentOf(part: string, total: string): number {
  const cents = (v: string) => {
    const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(v.trim());
    if (!m) return null;
    const value = BigInt(m[2] + (m[3] ?? "").padEnd(2, "0").slice(0, 2));
    return m[1] ? -value : value;
  };
  const p = cents(part);
  const t = cents(total);
  if (p === null || t === null || t <= 0n) return 0;
  const bp = (p * 10000n) / t;
  return Math.min(100, Math.max(0, Number(bp) / 100));
}
