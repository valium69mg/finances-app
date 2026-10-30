import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Loader2 } from "lucide-react";
import { incomeKeys, listIncome, type Income } from "../../api/income";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, rowButton, secondaryButton } from "../settings/ui";
import { describeIncomeError } from "./errors";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND } from "../../lib/tones";

interface Props {
  month: string;
  editingId: number | null;
  onEdit: (income: Income) => void;
}

export function IncomeList({ month, editingId, onEdit }: Props) {
  const list = useQuery({ queryKey: incomeKeys.list(month), queryFn: () => listIncome(month), retry: false });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando ingresos…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeIncomeError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    return <EmptyNote icon={KIND.Ingreso.icon}>No hay ingresos registrados en este periodo.</EmptyNote>;
  }

  return (
    <ul className="space-y-2">
      {list.data.map((i) => {
        const label = i.description || "Sin descripción";
        return (
          <li key={i.id}>
            <button type="button" onClick={() => onEdit(i)} aria-label={`Editar ${label}`} className={`${rowButton} ${editingId === i.id ? "bg-primary/5" : ""}`}>
              <span className="min-w-0 flex-1">
                <span className="block break-words font-medium">{label}</span>
                <span className="mt-0.5 block break-words text-sm text-muted">
                  {i.date} · {i.category || "Sin categoría"} · {i.payment_method}
                </span>
              </span>
              <span className="shrink-0 text-right">
                <span className="block font-semibold tabular-nums">
                  {formatMoney(i.amount)} {i.currency}
                </span>
                {i.currency !== "MXN" && (
                  <span className="block text-sm tabular-nums text-muted">
                    ≈ {formatMoney(i.amount_mxn)} MXN{i.exchange_rate ? ` (TC ${i.exchange_rate})` : ""}
                  </span>
                )}
              </span>
              <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
            </button>
          </li>
        );
      })}
    </ul>
  );
}
