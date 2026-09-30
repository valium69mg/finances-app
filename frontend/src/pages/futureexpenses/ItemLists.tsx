import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, Loader2, PiggyBank, Pencil, Trash2, ArrowRightLeft } from "lucide-react";
import { deleteFutureExpense, type FutureExpenseItem } from "../../api/futureExpenses";
import { formatMoney, percentOf } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeFutureError } from "./errors";
import { isEmptyBalance } from "./form";
import { useInvalidateFuture } from "./useInvalidate";

interface Props {
  items: FutureExpenseItem[];
  /** Free balance: the assign action only shows while there is something to assign. */
  freeBalance: string;
  editingId: number | null;
  onSaving: (item: FutureExpenseItem) => void;
  onAssign: (item: FutureExpenseItem) => void;
  onPay: (item: FutureExpenseItem) => void;
  onEdit: (item: FutureExpenseItem) => void;
  /** Called after an item was deleted so the page can drop a stale edit and show a notice. */
  onDeleted: (item: FutureExpenseItem) => void;
}

/** Active future expenses with their progress, the suggested monthly amount and the day-to-day actions. */
export function ActiveList({ items, freeBalance, editingId, onSaving, onAssign, onPay, onEdit, onDeleted }: Props) {
  const invalidate = useInvalidateFuture();
  const [confirming, setConfirming] = useState<number | null>(null);
  const remove = useMutation({
    mutationFn: (item: FutureExpenseItem) => deleteFutureExpense(item.id),
    onSuccess: async (_, item) => {
      setConfirming(null);
      onDeleted(item);
      await invalidate();
    },
  });

  return (
    <div className="space-y-3">
      {remove.isError && <ErrorBanner>{describeFutureError(remove.error)}</ErrorBanner>}
      <ul aria-label="Gastos futuros activos" className="divide-y divide-border rounded-xl border border-border">
        {items.map((f) => {
          const pct = percentOf(f.saved, f.target_amount);
          const covered = isEmptyBalance(f.remaining);
          return (
            <li key={f.id} className={editingId === f.id ? "bg-primary/5" : undefined}>
              <div className="space-y-3 p-4">
                <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
                  <p className="min-w-0 break-words font-medium">{f.name}</p>
                  <p className="text-sm text-muted">Vence el {dateLabel(f.due_date)}</p>
                </div>
                <div>
                  <div
                    role="progressbar"
                    aria-label={`Ahorro para ${f.name}`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={Math.round(pct)}
                    className="h-2.5 w-full overflow-hidden rounded-full bg-border"
                  >
                    <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
                  </div>
                  <p className="mt-1 text-sm tabular-nums">
                    {formatMoney(f.saved)} de {formatMoney(f.target_amount)} ({Math.round(pct)}%)
                  </p>
                  <p className="text-sm tabular-nums text-muted">
                    {covered ? "Meta cubierta." : `Aparta ${formatMoney(f.suggested_monthly)} al mes (${f.cycles_left} ${f.cycles_left === 1 ? "ciclo" : "ciclos"} restantes)`}
                  </p>
                </div>

                {confirming === f.id ? (
                  <div role="group" aria-label={`Confirmar eliminar ${f.name}`} className="flex flex-wrap items-center gap-2">
                    <span className="text-sm">¿Eliminar este gasto futuro? Su ahorro no se borra: vuelve al saldo libre.</span>
                    <button type="button" disabled={remove.isPending} onClick={() => remove.mutate(f)} className={dangerButton}>
                      {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                      Sí, eliminar
                    </button>
                    <button type="button" disabled={remove.isPending} onClick={() => setConfirming(null)} className={secondaryButton}>
                      Cancelar
                    </button>
                  </div>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    <button type="button" onClick={() => onSaving(f)} aria-label={`Agregar ahorro a ${f.name}`} className={secondaryButton}>
                      <PiggyBank className="h-4 w-4" aria-hidden="true" />
                      Agregar ahorro
                    </button>
                    {!isEmptyBalance(freeBalance) && (
                      <button type="button" onClick={() => onAssign(f)} aria-label={`Asignar saldo libre a ${f.name}`} className={secondaryButton}>
                        <ArrowRightLeft className="h-4 w-4" aria-hidden="true" />
                        Asignar saldo libre
                      </button>
                    )}
                    <button type="button" onClick={() => onPay(f)} aria-label={`Marcar pagado ${f.name}`} className={secondaryButton}>
                      <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                      Marcar pagado
                    </button>
                    <button type="button" onClick={() => onEdit(f)} aria-label={`Editar ${f.name}`} className={secondaryButton}>
                      <Pencil className="h-4 w-4" aria-hidden="true" />
                      Editar
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        remove.reset();
                        setConfirming(f.id);
                      }}
                      aria-label={`Eliminar ${f.name}`}
                      className={secondaryButton}
                    >
                      <Trash2 className="h-4 w-4" aria-hidden="true" />
                      Eliminar
                    </button>
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** Paid items, most recent first: what was paid and when. Deleting one never touches its registered expense. */
export function PaidList({ items, onDeleted }: { items: FutureExpenseItem[]; onDeleted: (item: FutureExpenseItem) => void }) {
  const invalidate = useInvalidateFuture();
  const [confirming, setConfirming] = useState<number | null>(null);
  const remove = useMutation({
    mutationFn: (item: FutureExpenseItem) => deleteFutureExpense(item.id),
    onSuccess: async (_, item) => {
      setConfirming(null);
      onDeleted(item);
      await invalidate();
    },
  });
  return (
    <div className="space-y-3">
      {remove.isError && <ErrorBanner>{describeFutureError(remove.error)}</ErrorBanner>}
      <ul aria-label="Gastos futuros pagados" className="divide-y divide-border rounded-xl border border-border">
        {items.map((f) => (
          <li key={f.id} className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <p className="break-words font-medium">{f.name}</p>
              <p className="text-sm text-muted">
                Pagado el {f.paid_at ? dateLabel(f.paid_at) : "—"} · {f.amount_paid ? formatMoney(f.amount_paid) : "—"} de {formatMoney(f.target_amount)} de meta
              </p>
            </div>
            {confirming === f.id ? (
              <div role="group" aria-label={`Confirmar eliminar ${f.name}`} className="flex flex-wrap items-center gap-2">
                <span className="text-sm">¿Quitarlo del historial? El gasto registrado se conserva.</span>
                <button type="button" disabled={remove.isPending} onClick={() => remove.mutate(f)} className={dangerButton}>
                  Sí, eliminar
                </button>
                <button type="button" disabled={remove.isPending} onClick={() => setConfirming(null)} className={secondaryButton}>
                  Cancelar
                </button>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => {
                  remove.reset();
                  setConfirming(f.id);
                }}
                aria-label={`Eliminar ${f.name}`}
                className={secondaryButton}
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" />
                Eliminar
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
