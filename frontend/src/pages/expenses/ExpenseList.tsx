import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Trash2 } from "lucide-react";
import { deleteExpense, expensesKeys, listExpenses, type Expense } from "../../api/expenses";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeExpenseError } from "./errors";
import { formatMoney } from "./money";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND } from "../../lib/tones";

interface Props {
  month: string;
  editingId: number | null;
  onEdit: (expense: Expense) => void;
  /** Called after an expense was deleted so the page can drop a stale edit. */
  onDeleted: (id: number) => void;
}

export function ExpenseList({ month, editingId, onEdit, onDeleted }: Props) {
  const qc = useQueryClient();
  const [confirmId, setConfirmId] = useState<number | null>(null);
  const list = useQuery({ queryKey: expensesKeys.list(month), queryFn: () => listExpenses(month), retry: false });
  const remove = useMutation({
    mutationFn: (id: number) => deleteExpense(id),
    onSuccess: (_, id) => {
      setConfirmId(null);
      onDeleted(id);
      return qc.invalidateQueries({ queryKey: expensesKeys.all });
    },
  });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando gastos…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeExpenseError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    return <EmptyNote icon={KIND.Gasto.icon}>No hay gastos registrados en este periodo.</EmptyNote>;
  }

  return (
    <div className="space-y-3">
      {remove.isError && <ErrorBanner>{describeExpenseError(remove.error)}</ErrorBanner>}
      <ul className="divide-y divide-border rounded-xl border border-border">
        {list.data.map((e) => {
          const label = e.description || "Sin descripción";
          const confirming = confirmId === e.id;
          return (
            <li key={e.id} className={`flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between ${editingId === e.id ? "bg-primary/5" : ""}`}>
              <div className="min-w-0 flex-1">
                <p className="break-words font-medium">{label}</p>
                <p className="mt-0.5 break-words text-sm text-muted">
                  {e.date} · {e.category || "Sin categoría"} · {e.payment_method}
                </p>
              </div>
              <div className="shrink-0 text-left sm:text-right">
                <p className="font-semibold tabular-nums">
                  {formatMoney(e.amount)} {e.currency}
                </p>
                {e.currency !== "MXN" && (
                  <p className="text-sm tabular-nums text-muted">
                    ≈ {formatMoney(e.amount_mxn)} MXN{e.exchange_rate ? ` (TC ${e.exchange_rate})` : ""}
                  </p>
                )}
              </div>
              {confirming ? (
                <div role="group" aria-label={`Confirmar eliminación de ${label}`} className="flex shrink-0 flex-wrap items-center gap-2">
                  <span className="text-sm">¿Eliminar este gasto?</span>
                  <button type="button" disabled={remove.isPending} onClick={() => remove.mutate(e.id)} className={dangerButton}>
                    {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                    Sí, eliminar
                  </button>
                  <button type="button" disabled={remove.isPending} onClick={() => setConfirmId(null)} className={secondaryButton}>
                    Cancelar
                  </button>
                </div>
              ) : (
                <div className="flex shrink-0 gap-2">
                  <button type="button" onClick={() => onEdit(e)} aria-label={`Editar ${label}`} className={secondaryButton}>
                    <Pencil className="h-4 w-4" aria-hidden="true" />
                    Editar
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      remove.reset();
                      setConfirmId(e.id);
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
