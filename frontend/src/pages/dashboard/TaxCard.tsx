import type { DashboardTax } from "../../api/dashboard";
import { formatMoney, formatRatePercent } from "../expenses/money";

/**
 * Estimated RESICO ISR of the month and its rate. It shows no payment status
 * yet: that belongs to the tax filing module and would be added here as a
 * further field of `tax` once the API carries it.
 */
export function TaxCard({ tax }: { tax: DashboardTax | null }) {
  return (
    <section aria-labelledby="tax-title" className="rounded-lg border border-border p-4">
      <h3 id="tax-title" className="font-semibold">
        ISR RESICO estimado
      </h3>
      {tax ? (
        <dl className="mt-1 space-y-1 text-sm">
          <div className="flex flex-wrap items-baseline justify-between gap-x-4">
            <dt className="text-muted">ISR del mes</dt>
            <dd className="font-medium tabular-nums">{formatMoney(tax.estimated_isr)}</dd>
          </div>
          <div className="flex flex-wrap items-baseline justify-between gap-x-4">
            <dt className="text-muted">Tasa</dt>
            <dd className="font-medium tabular-nums">{formatRatePercent(tax.rate)}</dd>
          </div>
        </dl>
      ) : (
        <p className="mt-1 text-sm text-muted">
          Sin estimación. Completa los rangos de RESICO en Configuración para calcular tu ISR del mes.
        </p>
      )}
    </section>
  );
}
