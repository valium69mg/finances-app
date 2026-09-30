import type { FutureExpenseItem, FutureExpensesList } from "../../api/futureExpenses";

/** Obviously fake examples: the repository is public, real data lives in production only. */
export const LAPTOP: FutureExpenseItem = {
  id: 1,
  name: "Laptop",
  target_amount: "8000.00",
  due_date: "2027-01-20",
  status: "active",
  saved: "2000.00",
  remaining: "6000.00",
  suggested_monthly: "1500.00",
  cycles_left: 4,
  paid_at: null,
  amount_paid: null,
  expense_movement_id: null,
  created_at: "2026-09-30T12:00:00Z",
  updated_at: "2026-09-30T12:00:00Z",
};

export const TRIP: FutureExpenseItem = {
  ...LAPTOP,
  id: 2,
  name: "Viaje",
  target_amount: "24000.00",
  due_date: "2026-12-15",
  saved: "24000.00",
  remaining: "0.00",
  suggested_monthly: "0.00",
  cycles_left: 3,
};

export const PAID: FutureExpenseItem = {
  ...LAPTOP,
  id: 3,
  name: "Teléfono",
  target_amount: "5000.00",
  due_date: "2026-09-01",
  status: "paid",
  saved: "0.00",
  remaining: "0.00",
  suggested_monthly: "0.00",
  cycles_left: 0,
  paid_at: "2026-09-02",
  amount_paid: "4800.00",
  expense_movement_id: 41,
};

export const LIST: FutureExpensesList = {
  active: [TRIP, LAPTOP],
  paid: [PAID],
  totals: { target: "32000.00", saved: "26000.00", remaining: "6000.00", suggested_monthly: "1500.00" },
  free_balance: "350.25",
};
