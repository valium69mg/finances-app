import { describe, expect, it } from "vitest";
import type { Category, General } from "../../api/settings";
import { budgetShare, categoryListWith, currentPeriodText, cycleStartLabel, formatMonthName, groupCategories, replaceAt } from "./summary";
import { fromCategoryDraft, newCategoryDraft, toCategoryDraft, toGeneralDraft, validateCategories, validateGeneralBlock } from "./validation";

const cat = (name: string, kind: string, budget: string | null = null): Category => ({ name, kind, budget, includes: "", keywords: [] });

describe("groupCategories", () => {
  it("orders Gasto, Ahorro, Ingreso, then other kinds, keeping the saved position", () => {
    const cats = [cat("Sueldo", "Ingreso"), cat("Renta", "Gasto"), cat("Viejo", "fixed"), cat("Fondo", "Ahorro"), cat("Comida", "Gasto")];
    const groups = groupCategories(cats);
    expect(groups.map((g) => g.kind)).toEqual(["Gasto", "Ahorro", "Ingreso", "fixed"]);
    expect(groups[0].items.map((i) => [i.category.name, i.index])).toEqual([
      ["Renta", 1],
      ["Comida", 4],
    ]);
  });

  it("leaves out empty groups", () => {
    expect(groupCategories([cat("Renta", "Gasto")]).map((g) => g.kind)).toEqual(["Gasto"]);
    expect(groupCategories([])).toEqual([]);
  });
});

describe("categoryListWith", () => {
  const saved = [cat("Renta", "Gasto", "100"), cat("Comida", "Gasto", "50")];

  it("replaces one category and keeps the rest untouched", () => {
    const edited = { ...toCategoryDraft(saved[0]), budget: "250.5" };
    const list = categoryListWith(saved, edited, 0);
    expect(list).toHaveLength(2);
    expect(fromCategoryDraft(list[0]).budget).toBe("250.5");
    expect(list[1]).toEqual(toCategoryDraft(saved[1]));
  });

  it("appends a new category at the end, trimmed when converted back", () => {
    const added = { ...newCategoryDraft(), name: "  Donaciones ", budget: "500" };
    const list = categoryListWith(saved, added, null);
    expect(list.map((d) => d.name)).toEqual(["Renta", "Comida", "  Donaciones "]);
    expect(fromCategoryDraft(list[2]).name).toBe("Donaciones");
  });

  it("feeds the cross-item validation: a name taken by another category and a total over the base", () => {
    const dup = { ...newCategoryDraft(), name: "renta" };
    expect(validateCategories(categoryListWith(saved, dup, null))["2.name"]).toBe("Ya existe una categoría con ese nombre.");
    const over = { ...toCategoryDraft(saved[0]), budget: "951" };
    expect(validateCategories(categoryListWith(saved, over, 0), "1000")["total"]).toContain("exceden por");
    expect(validateCategories(categoryListWith(saved, { ...over, budget: "950" }, 0), "1000")).toEqual({});
  });
});

describe("replaceAt", () => {
  it("returns a new list with only that item changed", () => {
    const list = [1, 2, 3];
    expect(replaceAt(list, 1, 9)).toEqual([1, 9, 3]);
    expect(list).toEqual([1, 2, 3]);
  });
});

describe("budgetShare", () => {
  it("gives the percentage and a capped width", () => {
    expect(budgetShare("3600", "62090.00")).toEqual({ percent: "5.8", width: 5.8 });
    expect(budgetShare("90000", "62090.00")?.width).toBe(100);
  });
  it("is null without a budget or a base", () => {
    expect(budgetShare(null, "100")).toBeNull();
    expect(budgetShare("", "100")).toBeNull();
    expect(budgetShare("10", null)).toBeNull();
  });
});

describe("period summary", () => {
  it("names the options", () => {
    expect(cycleStartLabel(0)).toBe("Mes calendario");
    expect(cycleStartLabel(31)).toBe("Último día del mes");
    expect(cycleStartLabel(32)).toBe("Último día hábil (lun–vie)");
    expect(cycleStartLabel(15)).toBe("Día 15");
  });
  it("previews the current period", () => {
    expect(currentPeriodText(31, "2026-10-31")).toBe("Periodo actual: 31 oct → 29 nov");
    expect(currentPeriodText(0, "2026-10-15")).toBe("Periodo actual: 1 oct → 31 oct");
    expect(currentPeriodText(32, "2026-10-29")).toBe("Periodo actual: 30 sep → 29 oct");
  });
});

describe("formatMonthName", () => {
  it("writes the month in Spanish", () => {
    expect(formatMonthName("2027-02")).toBe("febrero de 2027");
    expect(formatMonthName("nope")).toBe("nope");
  });
});

describe("validateGeneralBlock", () => {
  const general: General = {
    salary_usd: "2500",
    fx_rate_applied: "17.5",
    morse_fee_rate: "0.01",
    emergency_months: "6",
    extra_income_estimate_mxn: "5000",
    budget_includes_extra_income: false,
    extra_income_split: { sat_reserve_rate: "0.165", fondo_emergencia: "0.5", inversiones: "0.35", aguinaldo_vacaciones: "0.15" },
    investment_allocation: [
      { key: "VOO", value: "0.6" },
      { key: "BTC", value: "0.4" },
    ],
    cycle_start_day: 0,
  };

  it("keeps only the errors of the edited block", () => {
    const draft = { ...toGeneralDraft(general), salary_usd: "x", emergency_months: "" };
    expect(Object.keys(validateGeneralBlock(draft, "income"))).toEqual(["salary_usd"]);
    expect(Object.keys(validateGeneralBlock(draft, "extra"))).toEqual(["emergency_months"]);
    expect(validateGeneralBlock(draft, "split")).toEqual({});
  });

  it("reports the split and allocation sums in their own blocks", () => {
    const draft = toGeneralDraft(general);
    draft.extra_income_split.inversiones = "30";
    draft.investment_allocation[1].value = "30";
    expect(Object.keys(validateGeneralBlock(draft, "split"))).toEqual(["split"]);
    expect(Object.keys(validateGeneralBlock(draft, "alloc"))).toEqual(["alloc"]);
    expect(validateGeneralBlock(draft, "period")).toEqual({});
  });
});
