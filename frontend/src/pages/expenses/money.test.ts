import { describe, expect, it } from "vitest";
import { formatMoney, formatPercent, formatRatePercent, isNonZeroDecimal, isPositiveDecimal, percentOf, todayISO } from "./money";

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

describe("formatRatePercent", () => {
  it("shifts the decimal point without floats", () => {
    expect(formatRatePercent("0.015")).toBe("1.5%");
    expect(formatRatePercent("0.01")).toBe("1%");
    expect(formatRatePercent("0.0125")).toBe("1.25%");
    expect(formatRatePercent("0.0110")).toBe("1.1%");
    expect(formatRatePercent("0")).toBe("0%");
    expect(formatRatePercent("1")).toBe("100%");
    expect(formatRatePercent("x")).toBe("x");
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

describe("isNonZeroDecimal", () => {
  it("accepts positive and negative decimals but not zero or junk", () => {
    expect(isNonZeroDecimal("250")).toBe(true);
    expect(isNonZeroDecimal("-250.50")).toBe(true);
    expect(isNonZeroDecimal("0")).toBe(false);
    expect(isNonZeroDecimal("-0.00")).toBe(false);
    expect(isNonZeroDecimal("1,5")).toBe(false);
    expect(isNonZeroDecimal("-")).toBe(false);
  });
});

describe("formatPercent", () => {
  it("formats an existing percentage with two decimals", () => {
    expect(formatPercent("12.5")).toBe("12.50%");
    expect(formatPercent("-3.456")).toBe("-3.46%");
    expect(formatPercent("0")).toBe("0.00%");
    expect(formatPercent("x")).toBe("x");
  });
});

describe("percentOf", () => {
  it("computes a clamped share with integer math", () => {
    expect(percentOf("25", "100")).toBe(25);
    expect(percentOf("1", "3")).toBe(33.33);
    expect(percentOf("150", "100")).toBe(100);
    expect(percentOf("-5", "100")).toBe(0);
    expect(percentOf("5", "0")).toBe(0);
  });
});
