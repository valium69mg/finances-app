import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Link } from "react-router-dom";
import { listPendingPeriods, taxFilingKeys } from "../../api/taxFiling";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeTaxFilingError } from "./errors";
import { dateLabel, periodLabel } from "./labels";

/** Periods with issued invoices and no registered filing, each with a shortcut to declare it. */
export function PendingPeriods() {
  const pending = useQuery({ queryKey: taxFilingKeys.pending, queryFn: listPendingPeriods, retry: false });

  if (pending.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Buscando periodos pendientes…
      </p>
    );
  }
  if (pending.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeTaxFilingError(pending.error)}</ErrorBanner>
        <button type="button" onClick={() => void pending.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (pending.data.length === 0) {
    return <p className="text-sm text-muted">No tienes periodos pendientes de declarar: todos los meses con facturas emitidas ya tienen su declaración.</p>;
  }
  return (
    <ul aria-label="Periodos pendientes de declarar" className="divide-y divide-border rounded-xl border border-border">
      {pending.data.map((p) => (
        <li key={p.period} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className="flex flex-wrap items-center gap-2 font-medium">
              <span className="capitalize">{periodLabel(p.period)}</span>
              {p.overdue && (
                <span className="inline-block whitespace-nowrap rounded-full border border-destructive/50 bg-destructive/10 px-2.5 py-0.5 text-xs font-medium text-destructive">
                  Vencido
                </span>
              )}
            </p>
            <p className="text-sm text-muted">Fecha límite: {dateLabel(p.due_date)}</p>
          </div>
          <Link to={`/declaracion?period=${p.period}`} aria-label={`Declarar ${periodLabel(p.period)}`} className={`${secondaryButton} shrink-0`}>
            Declarar
          </Link>
        </li>
      ))}
    </ul>
  );
}
