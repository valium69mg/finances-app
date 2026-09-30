import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { Link } from "react-router-dom";
import { listUnfiledInvoices, taxFilingKeys } from "../../api/taxFiling";
import { formatMoney } from "../expenses/money";
import { periodLabel } from "./labels";

/**
 * Alert for issued invoices that no filing includes although their period was
 * already filed (typically issued after the declaration). Their income is not
 * part of any saved declaration; the app builds no complementary declaration,
 * so the user declares them with the SAT. Renders nothing when there are none
 * or while loading, and never blocks the rest of the page on an error.
 */
export function UnfiledInvoices() {
  const unfiled = useQuery({ queryKey: taxFilingKeys.unfiled, queryFn: listUnfiledInvoices, retry: false });
  if (!unfiled.data || unfiled.data.length === 0) return null;
  return (
    <section
      role="alert"
      aria-labelledby="unfiled-title"
      className="mt-6 space-y-3 rounded-xl border border-destructive/50 bg-destructive/10 p-4 text-sm sm:p-6"
    >
      <h2 id="unfiled-title" className="flex items-center gap-2 text-lg font-semibold tracking-tight text-destructive">
        <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
        Facturas emitidas en un periodo ya declarado
      </h2>
      <p>
        Estas facturas se emitieron después de registrar la declaración de su periodo, así que no están incluidas en ella y su ingreso todavía no está
        declarado. Decláralo con el SAT fuera de esta app (por ejemplo con una declaración complementaria).
      </p>
      <ul aria-label="Facturas sin declarar" className="divide-y divide-destructive/30 rounded-lg border border-destructive/30 bg-surface">
        {unfiled.data.map((inv) => (
          <li key={inv.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 p-3">
            <span className="min-w-0 break-words">
              <Link to="/facturas" className="focus-ring rounded font-medium underline">
                Factura #{inv.id}
              </Link>{" "}
              · {inv.client_id} · cobro {inv.collection_date}
            </span>
            <span className="tabular-nums">
              {formatMoney(inv.subtotal_mxn)} MXN · <span className="capitalize">{periodLabel(inv.period)}</span>
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
