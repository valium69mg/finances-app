import { describe, expect, it } from "vitest";
import { amountToPercent, baseIncome, distribution, formatPlain, isAtMostHundred, isHundred, percentToAmount, percentToRate, rateToPercent, sumDecimals } from "./planning";

describe("rate <-> percent conversion", () => {
  it.each([
    ["0.165", "16.5"],
    ["0.35", "35"],
    ["0.5", "50"],
    ["0.15", "15"],
    ["1.0", "100"],
    ["1", "100"],
    ["0", "0"],
    ["0.0125", "1.25"],
    ["0.001", "0.1"],
  ])("shows %s as %s%% and gives it back exactly", (rate, percent) => {
    expect(rateToPercent(rate)).toBe(percent);
    expect(percentToRate(percent)).toBe(rate === "1.0" ? "1" : rate);
  });

  it("round-trips the realistic split without float drift", () => {
    for (const rate of ["0.165", "0.35", "0.5", "0.15"]) expect(percentToRate(rateToPercent(rate))).toBe(rate);
  });

  it("converts what the user types", () => {
    expect(percentToRate("16.5")).toBe("0.165");
    expect(percentToRate("33.33")).toBe("0.3333");
    expect(percentToRate("100.0")).toBe("1");
    expect(percentToRate("007")).toBe("0.07");
  });

  it("leaves non-decimal text untouched", () => {
    expect(rateToPercent("abc")).toBe("abc");
    expect(percentToRate("12,5")).toBe("12,5");
    expect(percentToRate("")).toBe("");
  });
});

describe("percentages sum and range", () => {
  it("sums exactly", () => {
    expect(sumDecimals(["50", "35", "15"])).toBe("100");
    expect(sumDecimals(["33.33", "33.33", "33.34"])).toBe("100");
    expect(sumDecimals(["0.1", "0.2"])).toBe("0.3");
    expect(sumDecimals(["50", "abc"])).toBe("50");
  });

  it("detects exactly 100", () => {
    expect(isHundred("100")).toBe(true);
    expect(isHundred("100.00")).toBe(true);
    expect(isHundred("99.999")).toBe(false);
    expect(isHundred("100.01")).toBe(false);
    expect(isHundred("x")).toBe(false);
  });

  it("checks the 0-100 range", () => {
    expect(isAtMostHundred("100")).toBe(true);
    expect(isAtMostHundred("0")).toBe(true);
    expect(isAtMostHundred("100.01")).toBe(false);
    expect(isAtMostHundred("")).toBe(false);
  });
});

describe("baseIncome", () => {
  it("is salary * rate rounded to cents", () => {
    expect(baseIncome("3500", "17.74")).toBe("62090.00");
    expect(baseIncome("3500.55", "17.743")).toBe("62110.26");
    expect(baseIncome("2500.00", "17.50")).toBe("43750.00");
    expect(baseIncome("0.01", "0.5")).toBe("0.01"); // 0.005 rounds half up
  });

  it("is not available when a value is missing, zero or invalid", () => {
    expect(baseIncome("", "17.74")).toBeNull();
    expect(baseIncome("3500", "")).toBeNull();
    expect(baseIncome("0", "17.74")).toBeNull();
    expect(baseIncome("3500", "0.00")).toBeNull();
    expect(baseIncome("abc", "17")).toBeNull();
    expect(baseIncome("0.001", "0.001")).toBeNull(); // rounds to 0 cents
  });
});

describe("amount <-> percent of the base", () => {
  const base = "62090.00";
  it("shows one decimal, half up", () => {
    expect(amountToPercent("12000", base)).toBe("19.3");
    expect(amountToPercent("3600", base)).toBe("5.8");
    expect(amountToPercent("0", base)).toBe("0.0");
    expect(amountToPercent("62090", base)).toBe("100.0");
    expect(amountToPercent("70000", base)).toBe("112.7");
    expect(amountToPercent("", base)).toBeNull();
  });

  it("derives the amount from a percentage rounded to cents", () => {
    expect(percentToAmount("10", base)).toBe("6209.00");
    expect(percentToAmount("19.3", base)).toBe("11983.37");
    expect(percentToAmount("33.333", base)).toBe("20696.46");
    expect(percentToAmount("0", base)).toBe("0.00");
    expect(percentToAmount("x", base)).toBeNull();
  });
});

describe("distribution", () => {
  const lines = [
    { kind: "Gasto", budget: "3600" },
    { kind: "Gasto", budget: "2000" },
    { kind: "Ahorro", budget: "15000" },
    { kind: "Ahorro", budget: "" },
    { kind: "Ingreso", budget: "999999" }, // income has no budget
    { kind: "Gasto", budget: "abc" }, // invalid text counts as none
  ];

  it("adds Gasto and Ahorro budgets against the base", () => {
    const d = distribution(lines, "62090.00");
    expect(d).toMatchObject({ expense: "5600.00", saving: "15000.00", assigned: "20600.00", remaining: "41490.00", assignedPct: "33.2", remainingPct: "66.8", over: false, excess: "0.00" });
    expect(d!.expenseWidth + d!.savingWidth).toBeCloseTo(33.17, 1);
  });

  it("flags exactly 100% as not over and one cent more as over", () => {
    expect(distribution([{ kind: "Gasto", budget: "100.00" }], "100.00")).toMatchObject({ over: false, remaining: "0.00", assignedPct: "100.0" });
    const over = distribution([{ kind: "Gasto", budget: "100.01" }], "100.00");
    expect(over).toMatchObject({ over: true, excess: "0.01", remaining: "-0.01" });
    expect(over!.expenseWidth).toBe(100);
  });

  it("is null without a base", () => {
    expect(distribution(lines, "0.00")).toBeNull();
  });
});

describe("formatPlain", () => {
  it("drops zero cents and groups thousands", () => {
    expect(formatPlain("3500.00")).toBe("3,500");
    expect(formatPlain("17.74")).toBe("17.74");
    expect(formatPlain("17.5")).toBe("17.50");
    expect(formatPlain("62090.00")).toBe("62,090");
    expect(formatPlain("abc")).toBe("abc");
  });
});
