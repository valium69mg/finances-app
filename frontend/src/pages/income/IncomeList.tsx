import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Trash2 } from "lucide-react";
import { deleteIncome, incomeKeys, listIncome, type Income } from "../../api/income";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeIncomeError } from "./errors";

interface Props {
  month: string;
  editingId: number | null;
  onEdit: (income: Income) => void;
  /** Called after an income was deleted so the page can drop a stale edit. */
  onDeleted: (id: number) => void;
}

export function IncomeList({ month, editingId, onEdit, onDeleted }: Props) {
  const qc = useQueryClient();
  const [confirmId, setConfirmId] = useState<number | null>(null);
  const list = useQuery({ queryKey: incomeKeys.list(month), queryFn: () => listIncome(month), retry: false });
  const remove = useMutation({
    mutationFn: (id: number) => deleteIncome(id),
    onSuccess: (_, id) => {
      setConfirmId(null);
      onDeleted(id);
      return qc.invalidateQueries({ queryKey: incomeKeys.all });
    },
  });

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
    return <p className="text-sm text-muted">No hay ingresos registrados en este periodo.</p>;
  }

  return (
    <div className="space-y-3">
      {remove.isError && <ErrorBanner>{describeIncomeError(remove.error)}</ErrorBanner>}
      <ul className="divide-y divide-border rounded-xl border border-border">
        {list.data.map((i) => {
          const label = i.description || "Sin descripción";
          const confirming = confirmId === i.id;
          return (
            <li key={i.id} className={`flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between ${editingId === i.id ? "bg-primary/5" : ""}`}>
              <div className="min-w-0 flex-1">
                <p className="break-words font-medium">{label}</p>
                <p className="mt-0.5 break-words text-sm text-muted">
                  {i.date} · {i.category || "Sin categoría"} · {i.payment_method}
                </p>
              </div>
              <div className="shrink-0 text-left sm:text-right">
                <p className="font-semibold tabular-nums">
                  {formatMoney(i.amount)} {i.currency}
                </p>
                {i.currency !== "MXN" && (
                  <p className="text-sm tabular-nums text-muted">
                    ≈ {formatMoney(i.amount_mxn)} MXN{i.exchange_rate ? ` (TC ${i.exchange_rate})` : ""}
                  </p>
                )}
              </div>
              {confirming ? (
                <div role="group" aria-label={`Confirmar eliminación de ${label}`} className="flex shrink-0 flex-wrap items-center gap-2">
                  <span className="text-sm">¿Eliminar este ingreso?</span>
                  <button type="button" disabled={remove.isPending} onClick={() => remove.mutate(i.id)} className={dangerButton}>
                    {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                    Sí, eliminar
                  </button>
                  <button type="button" disabled={remove.isPending} onClick={() => setConfirmId(null)} className={secondaryButton}>
                    Cancelar
                  </button>
                </div>
              ) : (
                <div className="flex shrink-0 gap-2">
                  <button type="button" onClick={() => onEdit(i)} aria-label={`Editar ${label}`} className={secondaryButton}>
                    <Pencil className="h-4 w-4" aria-hidden="true" />
                    Editar
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      remove.reset();
                      setConfirmId(i.id);
                    }}
                    aria-label={`Eliminar ${label}`}
                    className={dangerButton}
                  >
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    Eliminar
                  </button>
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
