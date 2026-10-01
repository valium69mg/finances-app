import type { BudgetCheck, ExpenseRequest } from "../../api/expenseRequests";

/** Obviously fake examples: the repository is public, real data lives in production only. */
export const PENDING: ExpenseRequest = {
  id: 1,
  requester_email: "spouse@example.com",
  amount: "250.5",
  description: "Tacos",
  suggested_category: "Comida fuera",
  expense_date: "2026-10-03",
  status: "solicitada",
  decision_comment: null,
  decided_at: null,
  result_kind: null,
  result_movement_id: null,
  result_future_expense_id: null,
  created_at: "2026-10-03T12:00:00Z",
};

export const REJECTED: ExpenseRequest = {
  ...PENDING,
  id: 2,
  description: "Zapatos",
  status: "rechazada",
  decision_comment: "Mejor el mes que entra",
  decided_at: "2026-10-04T12:00:00Z",
};

export const APPROVED_EXPENSE: ExpenseRequest = {
  ...PENDING,
  id: 3,
  description: "Gasolina",
  status: "aprobada",
  decided_at: "2026-10-04T12:00:00Z",
  result_kind: "gasto",
  result_movement_id: 40,
};

export const APPROVED_FUTURE: ExpenseRequest = {
  ...PENDING,
  id: 4,
  description: "Regalo",
  status: "aprobada",
  decided_at: "2026-10-04T12:00:00Z",
  result_kind: "gasto_futuro",
  result_future_expense_id: 9,
};

export const CANCELLED: ExpenseRequest = { ...PENDING, id: 5, description: "Cine", status: "cancelada", decided_at: "2026-10-04T12:00:00Z" };

export const CATEGORIES = ["Mandado", "Comida fuera", "Transporte", "Ocio"];

/** 700 of a 1000 budget spent, a 200 request: fits. */
export const FITS: BudgetCheck = {
  category: "Comida fuera",
  budget: "1000.00",
  spent: "700.00",
  remaining: "300.00",
  amount: "200.00",
  projected_spent: "900.00",
  projected_remaining: "100.00",
  fits: true,
  over_by: "0.00",
};

/** 900 of a 1000 budget spent, a 250.50 request: exceeds by 150.50. */
export const EXCEEDS: BudgetCheck = {
  category: "Comida fuera",
  budget: "1000.00",
  spent: "900.00",
  remaining: "100.00",
  amount: "250.50",
  projected_spent: "1150.50",
  projected_remaining: "-150.50",
  fits: false,
  over_by: "150.50",
};

export const NO_BUDGET: BudgetCheck = {
  category: "Ocio",
  budget: null,
  spent: "40.00",
  remaining: null,
  amount: "250.50",
  projected_spent: "290.50",
  projected_remaining: null,
  fits: null,
  over_by: "0.00",
};
