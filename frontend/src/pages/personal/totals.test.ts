import { describe, expect, it } from "vitest";
import { isNegative, sumMoney } from "./totals";

describe("sumMoney", () => {
  it("adds decimals without float drift", () => {
    expect(sumMoney(["0.1", "0.2"])).toBe("0.30");
    expect(sumMoney(["1000.50", "250", "0.05"])).toBe("1250.55");
  });
  it("subtracts withdrawals", () => {
    expect(sumMoney(["1000.00", "-250.25"])).toBe("749.75");
    expect(sumMoney(["100", "-250.25"])).toBe("-150.25");
  });
  it("is zero for an empty list and ignores garbage", () => {
    expect(sumMoney([])).toBe("0.00");
    expect(sumMoney(["abc", "5"])).toBe("5.00");
  });
});

describe("isNegative", () => {
  it("detects a withdrawal", () => {
    expect(isNegative("-1.00")).toBe(true);
    expect(isNegative("1.00")).toBe(false);
    expect(isNegative("-0.00")).toBe(false);
  });
});
