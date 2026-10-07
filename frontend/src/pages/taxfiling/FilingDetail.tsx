import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Loader2, Trash2, Wallet, X } from "lucide-react";
import { deleteTaxFiling, getTaxFiling, taxFilingKeys, type Filing } from "../../api/taxFiling";
import { getSettings, settingsKeys } from "../../api/settings";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { Breakdown } from "./Breakdown";
import { describeTaxFilingError } from "./errors";
import { FilingDocuments } from "./FilingDocuments";
import { InvoiceRefs } from "./InvoiceRefs";
import { dateLabel, periodLabel } from "./labels";
import { FilingStatusBadge } from "./StatusBadge";
import { useInvalidateAfterFiling } from "./useInvalidate";

interface Props {
  period: string;
  onClose: () => void;
  onPay: (filing: Filing) => void;
  onDeleted: (period: string) => void;
}

/** One registered filing: its figures, payment, linked expense and included invoices. */
export function FilingDetail({ period, onClose, onPay, onDeleted }: Props) {
  const invalidate = useInvalidateAfterFiling();
  const [confirming, setConfirming] = useState(false);
  const detail = useQuery({ queryKey: taxFilingKeys.detail(period), queryFn: () => getTaxFiling(period), retry: false });
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const remove = useMutation({
    mutationFn: () => deleteTaxFiling(period),
    onSuccess: async () => {
      await invalidate();
      onDeleted(period);
    },
  });

  const filing = detail.data;
  return (
    <section aria-label={`Detalle de la declaración de ${periodLabel(period)}`} className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="flex flex-wrap items-center gap-2 text-lg font-semibold tracking-tight">
          <span className="capitalize">{periodLabel(period)}</span>
          {filing && <FilingStatusBadge status={filing.status} />}
        </h2>
        <button type="button" onClick={onClose} className={secondaryButton}>
          <X className="h-4 w-4" aria-hidden="true" />
          Cerrar detalle
        </button>
      </div>

      {detail.isPending && (
        <p role="status" className="flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Cargando declaración…
        </p>
      )}
      {detail.isError && (
        <div className="space-y-3">
          <ErrorBanner>{describeTaxFilingError(detail.error)}</ErrorBanner>
          <button type="button" onClick={() => void detail.refetch()} className={secondaryButton}>
            Reintentar
          </button>
        </div>
      )}
      {filing && (
        <div className="space-y-5">
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <div>
              <dt className="text-muted">Presentada el</dt>
              <dd className="font-medium">{dateLabel(filing.filing_date)}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted">Folio del acuse</dt>
              <dd className="break-all font-medium">{filing.folio || "Sin folio"}</dd>
            </div>
            {filing.payment ? (
              <>
                <div>
                  <dt className="text-muted">Pagada el</dt>
                  <dd className="font-medium">{dateLabel(filing.payment.date)}</dd>
                </div>
                <div>
                  <dt className="text-muted">ISR pagado</dt>
                  <dd className="font-medium tabular-nums">{formatMoney(filing.payment.isr_paid)}</dd>
                </div>
                <div>
                  <dt className="text-muted">IVA pagado</dt>
                  <dd className="font-medium tabular-nums">{formatMoney(filing.payment.iva_paid)}</dd>
                </div>
                <div>
                  <dt className="text-muted">Gasto de Impuestos</dt>
                  <dd className="font-medium">{filing.expense_movement_id !== null ? `Registrado (movimiento #${filing.expense_movement_id})` : "No se registró como gasto"}</dd>
                </div>
              </>
            ) : (
              <div className="sm:col-span-2">
                <dt className="text-muted">Pago</dt>
                <dd className="font-medium">Pendiente: aún no registras el pago al SAT.</dd>
              </div>
            )}
          </dl>

          <Breakdown amounts={filing} />

          <section aria-labelledby="detail-invoices-title" className="space-y-2">
            <h3 id="detail-invoices-title" className="text-base font-semibold tracking-tight">
              Facturas incluidas
            </h3>
            <InvoiceRefs invoices={filing.invoices} clients={settings.data?.clients ?? []} />
          </section>

          <FilingDocuments filing={filing} />

          {filing.status === "pendiente" && (
            <div className="space-y-3 border-t border-border pt-5">
              {remove.isError && <ErrorBanner>{describeTaxFilingError(remove.error)}</ErrorBanner>}
              {confirming ? (
                <div role="group" aria-label={`Confirmar eliminación de la declaración de ${periodLabel(period)}`} className="space-y-3">
                  <p className="text-sm">
                    ¿Eliminar este registro? Las facturas del periodo quedan sin declarar y podrás registrar la declaración de nuevo. Úsalo solo para corregir
                    un registro equivocado; no cancela nada en el SAT.
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <button type="button" disabled={remove.isPending} onClick={() => remove.mutate()} className={dangerButton}>
                      {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                      Sí, eliminar registro
                    </button>
                    <button type="button" disabled={remove.isPending} onClick={() => setConfirming(false)} className={secondaryButton}>
                      No, conservarlo
                    </button>
                  </div>
                </div>
              ) : (
                <div className="flex flex-wrap gap-2">
                  <button type="button" onClick={() => onPay(filing)} className={secondaryButton}>
                    <Wallet className="h-4 w-4" aria-hidden="true" />
                    Registrar pago
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      remove.reset();
                      setConfirming(true);
                    }}
                    className={dangerButton}
                  >
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    Eliminar registro
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
