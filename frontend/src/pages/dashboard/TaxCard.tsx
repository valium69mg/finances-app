import { Landmark } from "lucide-react";
import { Link } from "react-router-dom";
import type { DashboardTax } from "../../api/dashboard";
import { CardTitle } from "../../components/CardTitle";
import { EmptyNote } from "../../components/EmptyNote";
import { formatMoney, formatRatePercent } from "../expenses/money";
import { FilingStatusBadge } from "../taxfiling/StatusBadge";
import { periodLabel } from "../taxfiling/labels";

/**
 * Estimated RESICO ISR of the month and its rate, with the filing status of
 * the period: whether its declaration is unfiled, filed with the payment
 * pending or paid, and whether the previous period still needs action.
 */
export function TaxCard({ tax }: { tax: DashboardTax | null }) {
  return (
    <section aria-labelledby="tax-title" className="rounded-lg border border-border p-4">
      <CardTitle id="tax-title" icon={Landmark}>
        ISR RESICO estimado
      </CardTitle>
      {tax ? (
        <>
          <dl className="mt-3 space-y-1 text-sm">
            <div className="flex flex-wrap items-baseline justify-between gap-x-4">
              <dt className="text-muted">ISR del mes</dt>
              <dd className="font-medium tabular-nums">{formatMoney(tax.estimated_isr)}</dd>
            </div>
            <div className="flex flex-wrap items-baseline justify-between gap-x-4">
              <dt className="text-muted">Tasa</dt>
              <dd className="font-medium tabular-nums">{formatRatePercent(tax.rate)}</dd>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
              <dt className="text-muted">Declaración del mes</dt>
              <dd>
                <FilingStatusBadge status={tax.filing_status} />
              </dd>
            </div>
          </dl>
          {tax.previous_period_pending && (
            <p className="mt-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              La declaración o el pago de {periodLabel(tax.previous_period)} sigue pendiente.{" "}
              <Link to="/declaraciones-presentadas" className="font-medium underline">
                Ver declaraciones
              </Link>
            </p>
          )}
        </>
      ) : (
        <EmptyNote icon={Landmark}>Sin estimación. Completa los rangos de RESICO en Configuración para calcular tu ISR del mes.</EmptyNote>
      )}
    </section>
  );
}
