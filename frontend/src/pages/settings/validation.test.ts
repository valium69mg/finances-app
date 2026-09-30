import { describe, expect, it } from "vitest";
import {
  fromCategoryDraft,
  fromPauseDraft,
  isDecimal,
  isMonth,
  toCategoryDraft,
  validateBrackets,
  validateCategories,
  validatePause,
} from "./validation";

describe("isDecimal", () => {
  it.each(["0", "12", "1500.50", "0.0125"])("accepts %s", (v) => expect(isDecimal(v)).toBe(true));
  it.each(["", "-1", "1,5", "1e3", "abc", ".5", "1."])("rejects %j", (v) => expect(isDecimal(v)).toBe(false));
});

describe("isMonth", () => {
  it.each(["2026-10", "2027-01"])("accepts %s", (v) => expect(isMonth(v)).toBe(true));
  it.each(["2026-13", "2026-1", "26-10", ""])("rejects %j", (v) => expect(isMonth(v)).toBe(false));
});

describe("categories", () => {
  it("round-trips the budget as a string without touching precision", () => {
    const draft = toCategoryDraft({ name: "A", kind: "fixed", budget: "12345678901234.56", includes: "", keywords: ["x", "y"] });
    expect(fromCategoryDraft(draft).budget).toBe("12345678901234.56");
    expect(draft.keywords).toBe("x, y");
  });

  it("maps an empty budget to null and splits keywords", () => {
    const out = fromCategoryDraft({ name: "A", kind: "fixed", budget: " ", includes: "", keywords: "a, b,, c " });
    expect(out.budget).toBeNull();
    expect(out.keywords).toEqual(["a", "b", "c"]);
  });

  it("flags an invalid budget by row", () => {
    const errors = validateCategories([
      { name: "A", kind: "k", budget: "10", includes: "", keywords: "" },
      { name: "B", kind: "k", budget: "abc", includes: "", keywords: "" },
      { name: "C", kind: "k", budget: "", includes: "", keywords: "" },
    ]);
    expect(Object.keys(errors)).toEqual(["1.budget"]);
  });
});

describe("brackets", () => {
  it("requires at least one row and valid decimals", () => {
    expect(validateBrackets([])).toHaveProperty("rows");
    expect(validateBrackets([{ upper: "1000", rate: "x" }])).toEqual({ "0.rate": expect.any(String) });
  });
});

describe("pause", () => {
  const base = { monthsText: "2026-10, 2026-11", normal_budget: "5000", resume_month: "2027-02", plan: { "2026-10": "12000", "2026-11": "12000" }, note: "" };

  it("accepts a valid draft", () => {
    expect(validatePause(base)).toEqual({});
  });

  it("rejects malformed and duplicated months and missing plan amounts", () => {
    expect(validatePause({ ...base, monthsText: "2026-10, oct" })).toHaveProperty("months");
    expect(validatePause({ ...base, monthsText: "2026-10, 2026-10" })).toHaveProperty("months");
    expect(validatePause({ ...base, plan: { "2026-10": "12000" } })).toHaveProperty(["plan.2026-11"]);
  });

  it("sends the plan only for months still paused", () => {
    const out = fromPauseDraft({ ...base, monthsText: "2026-10", plan: { ...base.plan, "2026-12": "1" } });
    expect(out.months).toEqual(["2026-10"]);
    expect(out.future_expenses_plan).toEqual({ "2026-10": "12000" });
  });
});
