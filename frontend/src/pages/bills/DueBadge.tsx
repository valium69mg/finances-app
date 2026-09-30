import type { Bill } from "../../api/bills";
import { dueState, type DueKind } from "./labels";

const STYLE: Record<Exclude<DueKind, "none">, string> = {
  overdue: "border-destructive/50 bg-destructive/10 text-destructive",
  today: "border-accent/60 bg-accent/15 text-foreground",
  soon: "border-accent/60 bg-accent/15 text-foreground",
};

/** Overdue / due-soon state as text on a tinted pill; the color never carries the meaning alone. */
export function DueBadge({ bill }: { bill: Pick<Bill, "active" | "overdue" | "due_soon" | "days_until_due"> }) {
  const state = dueState(bill);
  if (state.kind === "none") return null;
  return <span className={`inline-block whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium ${STYLE[state.kind]}`}>{state.label}</span>;
}
