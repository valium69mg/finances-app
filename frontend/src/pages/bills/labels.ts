import type { Bill, OccurrenceStatus, Recurrence } from "../../api/bills";

export const RECURRENCES: Recurrence[] = ["weekly", "biweekly", "monthly", "bimonthly", "yearly"];

/** Spanish text of a recurrence. */
export const RECURRENCE_LABEL: Record<Recurrence, string> = {
  weekly: "Semanal",
  biweekly: "Cada dos semanas",
  monthly: "Mensual",
  bimonthly: "Bimestral",
  yearly: "Anual",
};

/** Spanish text of the state of an occurrence in the history. */
export const OCCURRENCE_STATUS_LABEL: Record<OccurrenceStatus, string> = {
  pending: "Pendiente",
  paid: "Pagado",
  skipped: "Omitido",
};

export type DueKind = "overdue" | "today" | "soon" | "none";

export interface DueState {
  kind: DueKind;
  /** Spanish badge text; empty when the bill needs no attention. */
  label: string;
}

const days = (n: number) => `${n} ${n === 1 ? "día" : "días"}`;

/**
 * Attention state of a bill's pending occurrence from the flags the API
 * computes. An inactive bill or one without a pending occurrence never raises a
 * badge. Overdue takes precedence over due soon.
 */
export function dueState(bill: Pick<Bill, "active" | "overdue" | "due_soon" | "days_until_due">): DueState {
  const n = bill.days_until_due;
  if (!bill.active || n === null) return { kind: "none", label: "" };
  if (bill.overdue) return { kind: "overdue", label: `Vencido hace ${days(Math.abs(n))}` };
  if (bill.due_soon) {
    if (n === 0) return { kind: "today", label: "Vence hoy" };
    return { kind: "soon", label: n === 1 ? "Vence mañana" : `Vence en ${days(n)}` };
  }
  return { kind: "none", label: "" };
}

/** Spanish text of the reminder lead time. */
export function leadLabel(daysBefore: number): string {
  if (daysBefore === 0) return "Avisar el mismo día";
  return `Avisar ${days(daysBefore)} antes`;
}
