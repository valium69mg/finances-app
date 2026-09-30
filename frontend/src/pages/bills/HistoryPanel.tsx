import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { billsKeys, getBill, type Occurrence } from "../../api/bills";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeBillsError } from "./errors";
import { OCCURRENCE_STATUS_LABEL } from "./labels";

function detail(o: Occurrence): string {
  if (o.status === "paid") {
    const amount = o.amount_paid ? `${formatMoney(o.amount_paid)} ${o.currency ?? ""}`.trim() : "";
    const expense = o.expense_movement_id === null ? "el gasto ya no existe" : `gasto #${o.expense_movement_id}`;
    return `Pagado el ${o.paid_on ?? ""}${amount ? ` · ${amount}` : ""} · ${expense}`;
  }
  return "Omitido: no se registró ningún gasto";
}

/** Paid and skipped occurrences of one bill, newest first. */
export function HistoryPanel({ billId, name }: { billId: number; name: string }) {
  const query = useQuery({ queryKey: billsKeys.detail(billId), queryFn: () => getBill(billId), retry: false });

  if (query.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando historial…
      </p>
    );
  }
  if (query.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeBillsError(query.error)}</ErrorBanner>
        <button type="button" onClick={() => void query.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (query.data.history.length === 0) {
    return <p className="text-sm text-muted">Aún no hay pagos ni omisiones de este pago recurrente.</p>;
  }
  return (
    <ul aria-label={`Historial de ${name}`} className="divide-y divide-border rounded-lg border border-border">
      {query.data.history.map((o) => (
        <li key={o.id} className="flex flex-col gap-1 px-3 py-2 text-sm sm:flex-row sm:items-center sm:justify-between">
          <span className="font-medium tabular-nums">{o.due_date}</span>
          <span className="min-w-0 break-words text-muted">
            <span className="font-medium text-foreground">{OCCURRENCE_STATUS_LABEL[o.status]}</span> · {detail(o)}
          </span>
        </li>
      ))}
    </ul>
  );
}
