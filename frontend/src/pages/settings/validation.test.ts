import { describe, expect, it } from "vitest";
import {
  fromCategoryDraft,
  fromPauseDraft,
  isDecimal,
  isMonth,
  toCategoryDraft,
  allocationTotal,
  fromGeneralDraft,
  splitDestinationsTotal,
  toGeneralDraft,
  validateBrackets,
  validateCategories,
  validateClients,
  validateGeneral,
  validatePause,
} from "./validation";
import type { Client, General } from "../../api/settings";

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

describe("general", () => {
  const general = (over: Partial<General> = {}): General => ({
    salary_usd: "1000",
    fx_rate_applied: "17.5",
    morse_fee_rate: "0.01",
    emergency_months: "6",
    extra_income_estimate_mxn: "0",
    budget_includes_extra_income: false,
    extra_income_split: {},
    investment_allocation: [],
    cycle_start_day: 0,
    ...over,
  });

  it.each([0, 31])("accepts cycle_start_day %i", (d) => expect(validateGeneral(general({ cycle_start_day: d }))).toEqual({}));
  it.each([-1, 32, 1.5])("rejects cycle_start_day %j", (d) => expect(validateGeneral(general({ cycle_start_day: d }))).toHaveProperty("cycle_start_day"));
});

describe("general percentages", () => {
  const realistic = (over: Partial<General> = {}): General => ({
    salary_usd: "3500",
    fx_rate_applied: "17.74",
    morse_fee_rate: "0.001",
    emergency_months: "6",
    extra_income_estimate_mxn: "35000",
    budget_includes_extra_income: false,
    extra_income_split: { sat_reserve_rate: "0.165", fondo_emergencia: "0.5", inversiones: "0.35", aguinaldo_vacaciones: "0.15" },
    investment_allocation: [{ key: "voo", value: "1.0" }],
    cycle_start_day: 0,
    ...over,
  });

  it("shows decimals as percentages and saves them back exactly", () => {
    const draft = toGeneralDraft(realistic());
    expect(draft.extra_income_split).toEqual({ sat_reserve_rate: "16.5", fondo_emergencia: "50", inversiones: "35", aguinaldo_vacaciones: "15" });
    expect(draft.investment_allocation).toEqual([{ key: "voo", value: "100" }]);
    const saved = fromGeneralDraft(draft);
    expect(saved.extra_income_split).toEqual({ sat_reserve_rate: "0.165", fondo_emergencia: "0.5", inversiones: "0.35", aguinaldo_vacaciones: "0.15" });
    expect(saved.investment_allocation).toEqual([{ key: "voo", value: "1" }]);
  });

  it("accepts today's realistic values", () => {
    expect(validateGeneral(toGeneralDraft(realistic()))).toEqual({});
  });

  it("requires the three destinations to add up to exactly 100 %", () => {
    const draft = toGeneralDraft(realistic());
    const off = { ...draft, extra_income_split: { ...draft.extra_income_split, aguinaldo_vacaciones: "14.9" } };
    expect(validateGeneral(off).split).toContain("99.9 %");
    const over = { ...draft, extra_income_split: { ...draft.extra_income_split, inversiones: "36" } };
    expect(validateGeneral(over).split).toContain("101 %");
  });

  it("keeps the SAT reserve out of the sum and only checks its range", () => {
    const draft = toGeneralDraft(realistic());
    expect(validateGeneral({ ...draft, extra_income_split: { ...draft.extra_income_split, sat_reserve_rate: "90" } })).toEqual({});
    expect(validateGeneral({ ...draft, extra_income_split: { ...draft.extra_income_split, sat_reserve_rate: "100.5" } })).toHaveProperty("split.sat_reserve_rate");
  });

  it("flags invalid text and out-of-range percentages by field, without a sum error on top", () => {
    const draft = toGeneralDraft(realistic());
    const bad = { ...draft, extra_income_split: { ...draft.extra_income_split, inversiones: "3,5", fondo_emergencia: "150" } };
    const e = validateGeneral(bad);
    expect(e["split.inversiones"]).toBeTruthy();
    expect(e["split.fondo_emergencia"]).toBeTruthy();
    expect(e).not.toHaveProperty("split");
  });

  it("requires the investment allocation to add up to 100 %", () => {
    const draft = toGeneralDraft(realistic({ investment_allocation: [{ key: "voo", value: "0.6" }, { key: "vxus", value: "0.4" }] }));
    expect(validateGeneral(draft)).toEqual({});
    const off = { ...draft, investment_allocation: [{ key: "voo", value: "60" }, { key: "vxus", value: "30" }] };
    expect(validateGeneral(off).alloc).toContain("90 %");
    expect(allocationTotal([])).toBeNull();
  });

  it("sums absent destinations at their defaults like the API", () => {
    expect(splitDestinationsTotal({})).toBeNull();
    expect(splitDestinationsTotal({ inversiones: "35" })).toBe("100");
    expect(splitDestinationsTotal({ inversiones: "90" })).toBe("155");
  });
});

describe("category budget total against the base", () => {
  const cat = (name: string, kind: string, budget: string) => ({ name, kind, budget, includes: "", keywords: "" });

  it("blocks when the Gasto and Ahorro budgets exceed the base and names the excess", () => {
    const e = validateCategories([cat("A", "Gasto", "40000"), cat("B", "Ahorro", "22090.01")], "62090.00");
    expect(e.total).toContain("$0.01");
    expect(e.total).toContain("$62,090.00");
  });

  it("accepts exactly 100 %, ignores Ingreso and skips the rule without a base", () => {
    expect(validateCategories([cat("A", "Gasto", "40000"), cat("B", "Ahorro", "22090"), cat("C", "Ingreso", "999999")], "62090.00")).toEqual({});
    expect(validateCategories([cat("A", "Gasto", "999999")], null)).toEqual({});
  });

  it("reports the invalid amount instead of a total", () => {
    const e = validateCategories([cat("A", "Gasto", "1,5"), cat("B", "Gasto", "99999999")], "100.00");
    expect(e).toHaveProperty("0.budget");
    expect(e).not.toHaveProperty("total");
  });
});

describe("clients", () => {
  const client = (postal_code: string): Client => ({
    id: "ibl", name: "IBL", type: "", currency: "MXN", iva_rate: "0.16", rfc: "", regimen: "", uso_cfdi: "", ret_isr_rate: "0.0125",
    ret_iva_rate: "0.106667", concepto: "", clave_prod_serv: "", clave_unidad: "", address: "", tax_residence: "", contract: "",
    real_payer: "", postal_code,
  });

  it.each(["", "  ", "06600", "64000"])("accepts the postal code %j", (v) => expect(validateClients([client(v)])).toEqual({}));
  it.each(["6600", "066000", "6600A", "06 600"])("rejects the postal code %j", (v) =>
    expect(validateClients([client(v)])).toEqual({ "0.postal_code": "El código postal debe tener 5 dígitos o dejarse vacío." }),
  );
});
