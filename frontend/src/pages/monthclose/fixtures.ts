import type { MonthClose } from "../../api/monthClose";

/** A close with an over-budget category, a leftover sent to the emergency fund, two hints and a pending filing. */
export const CLOSE: MonthClose = {
  period: "2026-09",
  closed_at: "2026-10-02T15:04:05Z",
  categories: [
    { category: "Renta", spent: "12000.50", budget: "12000.50", remaining: "0.00", over_budget: false },
    { category: "Comida", spent: "9000.00", budget: "6000", remaining: "-3000.00", over_budget: true },
    { category: "Servicios", spent: "1000.00", budget: "2000", remaining: "1000.00", over_budget: false },
    { category: "Suscripciones", spent: "139.00", budget: null, remaining: null, over_budget: false },
  ],
  income: "30000.00",
  expenses: "22139.50",
  savings: "1000.00",
  available: "6860.50",
  emergency: { accumulated: "20000.00", goal: "60000.00" },
  suggestion: { to_emergency_fund: "6860.50", to_investments: "0", to_future_expenses: "0", investments_paused: false },
  adjustments: [
    { category: "Comida", kind: "Gasto", budget: "6000", real: "9000.00", deviation_pct: "50.00" },
    { category: "Servicios", kind: "Gasto", budget: "2000", real: "1000.00", deviation_pct: "-50.00" },
    { category: "Inversiones", kind: "Ahorro", budget: "5000", real: "1000.00", deviation_pct: "-80" },
  ],
  tax_filing_status: "pendiente",
};
