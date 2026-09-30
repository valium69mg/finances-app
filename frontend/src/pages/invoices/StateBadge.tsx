import type { InvoiceState } from "../../api/invoices";
import { STATE_LABEL } from "./labels";

const STYLE: Record<InvoiceState, string> = {
  preparada: "border-border bg-background text-foreground",
  emitida: "border-accent/60 bg-accent/15 text-foreground",
  cancelada: "border-destructive/50 bg-destructive/10 text-destructive",
};

/** Invoice state as text on a tinted pill; the color never carries the meaning alone. */
export function StateBadge({ state }: { state: InvoiceState }) {
  return (
    <span className={`inline-block whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium ${STYLE[state]}`}>
      {STATE_LABEL[state] ?? state}
    </span>
  );
}
