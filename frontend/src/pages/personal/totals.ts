/**
 * Sum of decimal strings as a decimal string with 2 decimals, computed in integer cents so no float is involved.
 * Values that are not plain decimals are ignored. Extra decimals are truncated (the backend sends at most 2).
 */
export function sumMoney(values: string[]): string {
  let cents = 0n;
  for (const v of values) {
    const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(v.trim());
    if (!m) continue;
    const c = BigInt(m[2] + (m[3] ?? "").padEnd(2, "0").slice(0, 2));
    cents += m[1] ? -c : c;
  }
  const sign = cents < 0n ? "-" : "";
  const abs = (cents < 0n ? -cents : cents).toString().padStart(3, "0");
  return `${sign}${abs.slice(0, -2)}.${abs.slice(-2)}`;
}

/** True for a negative decimal string such as "-500.00" (an Ahorro withdrawal). */
export const isNegative = (v: string) => /^-\s*\d*\.?\d*[1-9]/.test(v.trim());
