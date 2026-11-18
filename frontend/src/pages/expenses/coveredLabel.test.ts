import { describe, expect, it } from "vitest";
import type { Expense } from "../../api/expenses";
import { coveredLabel } from "./ExpenseList";

const expense = (over: Partial<Expense>): Expense => ({
  id: 1, date: "2026-10-05", description: "x", category: "Ocio", payment_method: "Débito", currency: "MXN", amount: "500.00", exchange_rate: null, amount_mxn: "500.00", ...over,
});

describe("coveredLabel", () => {
  it("is empty when savings paid nothing", () => {
    expect(coveredLabel(expense({}))).toBeNull();
    expect(coveredLabel(expense({ covered_by_savings: "0" }))).toBeNull();
  });
  it("is the plain badge when savings paid all of it", () => {
    expect(coveredLabel(expense({ covered_by_savings: "500.00" }))).toBe("Pagado con ahorro");
  });
  it("shows the covered amount when only part was paid from savings", () => {
    expect(coveredLabel(expense({ covered_by_savings: "300.00" }))).toBe("Pagado con ahorro: $300.00");
  });
});
