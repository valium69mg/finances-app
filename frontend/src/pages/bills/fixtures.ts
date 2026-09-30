import type { Bill, BillDetail, Occurrence } from "../../api/bills";

const pending = (id: number, due: string): Occurrence => ({
  id,
  due_date: due,
  status: "pending",
  paid_on: null,
  expense_movement_id: null,
  amount_paid: null,
  currency: null,
  resolved_at: null,
});

/** A monthly fixed-amount bill that is overdue. */
export const OVERDUE_BILL: Bill = {
  id: 1,
  name: "Megacable",
  category: "Servicios",
  amount: "550.00",
  currency: "MXN",
  recurrence: "monthly",
  next_due_date: "2026-10-01",
  reminder_lead_days: 3,
  active: true,
  notes: "",
  created_at: "2026-09-01T12:00:00Z",
  pending_occurrence: pending(10, "2026-10-01"),
  overdue: true,
  due_soon: false,
  days_until_due: -9,
};

/** A bimonthly variable bill (no fixed amount) due in two days. */
export const VARIABLE_BILL: Bill = {
  ...OVERDUE_BILL,
  id: 2,
  name: "Luz",
  amount: null,
  recurrence: "bimonthly",
  next_due_date: "2026-10-12",
  notes: "Recibo CFE",
  pending_occurrence: pending(20, "2026-10-12"),
  overdue: false,
  due_soon: true,
  days_until_due: 2,
};

/** An upcoming bill that needs no attention yet. */
export const FUTURE_BILL: Bill = {
  ...OVERDUE_BILL,
  id: 3,
  name: "Netflix",
  category: "Suscripciones",
  amount: "139.00",
  next_due_date: "2026-11-20",
  pending_occurrence: pending(30, "2026-11-20"),
  overdue: false,
  due_soon: false,
  days_until_due: 41,
};

export const INACTIVE_BILL: Bill = { ...FUTURE_BILL, id: 4, name: "Disney+", active: false };

export const DETAIL: BillDetail = {
  ...OVERDUE_BILL,
  history: [
    { id: 9, due_date: "2026-09-01", status: "paid", paid_on: "2026-09-03", expense_movement_id: 42, amount_paid: "499.50", currency: "MXN", resolved_at: "2026-09-03T10:00:00Z" },
    { id: 8, due_date: "2026-08-01", status: "skipped", paid_on: null, expense_movement_id: null, amount_paid: null, currency: null, resolved_at: "2026-08-05T10:00:00Z" },
    { id: 7, due_date: "2026-07-01", status: "paid", paid_on: "2026-07-01", expense_movement_id: null, amount_paid: "550.00", currency: "MXN", resolved_at: "2026-07-01T10:00:00Z" },
  ],
};
