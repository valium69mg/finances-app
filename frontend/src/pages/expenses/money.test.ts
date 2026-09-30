import { describe, expect, it } from "vitest";
import { formatMoney, isPositiveDecimal, todayISO } from "./money";

describe("formatMoney", () => {
  it("pads, groups and rounds decimal strings without floats", () => {
    expect(formatMoney("1000")).toBe("$1,000.00");
    expect(formatMoney("62.5")).toBe("$62.50");
    expect(formatMoney("1100.4122")).toBe("$1,100.41");
    expect(formatMoney("0.005")).toBe("$0.01");
    expect(formatMoney("9007199254740993.99")).toBe("$9,007,199,254,740,993.99");
  });

  it("keeps the sign and leaves non-decimals alone", () => {
    expect(formatMoney("-100")).toBe("-$100.00");
    expect(formatMoney("-0.001")).toBe("$0.00");
    expect(formatMoney("abc")).toBe("abc");
  });
});

describe("isPositiveDecimal", () => {
  it("accepts plain positive decimals only", () => {
    expect(isPositiveDecimal("12.50")).toBe(true);
    expect(isPositiveDecimal("0")).toBe(false);
    expect(isPositiveDecimal("12,5")).toBe(false);
    expect(isPositiveDecimal("-3")).toBe(false);
    expect(isPositiveDecimal("")).toBe(false);
  });
});

describe("todayISO", () => {
  it("formats local dates with zero padding", () => {
    expect(todayISO(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});
