import { AlertTriangle } from "lucide-react";
import type { InvoiceWarning } from "../../api/invoices";
import { describeWarning } from "./labels";

/** Non-blocking findings of the last operation (possible duplicate, XML mismatch). */
export function WarningsList({ warnings, currency }: { warnings: InvoiceWarning[]; currency: string }) {
  if (warnings.length === 0) return null;
  return (
    <div role="status" aria-label="Advertencias" className="space-y-2 rounded-lg border border-border bg-primary/5 p-3 text-sm">
      <p className="flex items-center gap-2 font-medium">
        <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
        {warnings.length === 1 ? "Advertencia" : "Advertencias"}
      </p>
      <ul className="list-disc space-y-1 pl-5">
        {warnings.map((w, i) => (
          <li key={`${w.code}-${i}`} className="break-words">
            {describeWarning(w, currency)}
          </li>
        ))}
      </ul>
    </div>
  );
}
