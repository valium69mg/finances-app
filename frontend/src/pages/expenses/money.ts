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
