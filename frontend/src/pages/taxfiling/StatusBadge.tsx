import type { PeriodFilingStatus } from "../../api/taxFiling";
import { FILING_STATUS_LABEL } from "./labels";

const STYLE: Record<PeriodFilingStatus, string> = {
  ninguna: "border-border bg-background text-foreground",
  pendiente: "border-destructive/50 bg-destructive/10 text-destructive",
  pagada: "border-accent/60 bg-accent/15 text-foreground",
};

/** Filing payment state as text on a tinted pill; the color never carries the meaning alone. */
export function FilingStatusBadge({ status }: { status: PeriodFilingStatus }) {
  return (
    <span className={`inline-block whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium ${STYLE[status]}`}>
      {FILING_STATUS_LABEL[status] ?? status}
    </span>
  );
}
