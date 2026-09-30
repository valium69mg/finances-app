import type { TaxInvoiceRef, TaxWarning } from "../../api/taxFiling";
import type { Client } from "../../api/settings";
import { AlertTriangle } from "lucide-react";
import { StateBadge } from "../invoices/StateBadge";
import { clientName } from "../invoices/labels";
import { formatMoney } from "../expenses/money";
import { describeTaxWarning } from "./labels";

/** Invoices included in a declaration, oldest first. */
export function InvoiceRefs({ invoices, clients }: { invoices: TaxInvoiceRef[]; clients: Client[] }) {
  if (invoices.length === 0) {
    return <p className="text-sm text-muted">No hay facturas emitidas en este periodo, así que la declaración queda en ceros.</p>;
  }
  return (
    <ul aria-label="Facturas incluidas" className="divide-y divide-border rounded-xl border border-border">
      {invoices.map((inv) => (
        <li key={inv.id} className="flex flex-col gap-1 p-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
          <div className="min-w-0">
            <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
              <span>Factura #{inv.id}</span>
              <StateBadge state={inv.state} />
            </p>
            <p className="break-words text-sm text-muted">
              {clientName(clients, inv.client_id)} · cobrada el {inv.collection_date}
            </p>
            {inv.uuid && <p className="break-all text-xs text-muted">UUID {inv.uuid}</p>}
          </div>
          <p className="shrink-0 text-sm font-medium tabular-nums">{formatMoney(inv.subtotal_mxn)} sin IVA</p>
        </li>
      ))}
    </ul>
  );
}

/** Non-blocking notices about the period (prepared invoices left out, period already filed). */
export function TaxWarnings({ warnings }: { warnings: TaxWarning[] }) {
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
            {describeTaxWarning(w)}
          </li>
        ))}
      </ul>
    </div>
  );
}
