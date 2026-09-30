import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Trash2 } from "lucide-react";
import { SAVINGS_LIST_LIMIT, deleteSaving, listSavings, savingsKeys, type Saving } from "../../api/savings";
import type { Instrument } from "../../api/settings";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";

interface Props {
  month: string;
  instruments: Instrument[];
  editingId: number | null;
  onEdit: (saving: Saving) => void;
  /** Called after a saving was deleted so the page can drop a stale edit. */
  onDeleted: (id: number) => void;
}

export function SavingsList({ month, instruments, editingId, onEdit, onDeleted }: Props) {
  const qc = useQueryClient();
  const [confirmId, setConfirmId] = useState<number | null>(null);
  const list = useQuery({ queryKey: savingsKeys.list(month), queryFn: () => listSavings(month), retry: false });
  const remove = useMutation({
    mutationFn: (id: number) => deleteSaving(id),
    onSuccess: (_, id) => {
      setConfirmId(null);
      onDeleted(id);
      return qc.invalidateQueries({ queryKey: savingsKeys.all });
    },
  });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando ahorros…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeSavingsError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    return <p className="text-sm text-muted">No hay ahorros registrados en este mes.</p>;
  }

  return (
    <div className="space-y-3">
      {remove.isError && <ErrorBanner>{describeSavingsError(remove.error)}</ErrorBanner>}
      {list.data.length >= SAVINGS_LIST_LIMIT && (
        <p role="status" className="text-sm text-muted">
          Mostrando los {SAVINGS_LIST_LIMIT} más recientes de este mes; los anteriores no aparecen en la lista.
        </p>
      )}
      <ul className="divide-y divide-border rounded-xl border border-border">
        {list.data.map((s) => {
          const label = s.description || "Sin descripción";
          const instrument = instruments.find((i) => i.id === s.instrument)?.name ?? s.instrument;
          const confirming = confirmId === s.id;
          const withdrawal = s.amount.trim().startsWith("-");
          const isTransfer = Boolean(s.transfer_id);
          return (
            <li key={s.id} className={`flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between ${editingId === s.id ? "bg-primary/5" : ""}`}>
              <div className="min-w-0 flex-1">
                <p className="break-words font-medium">
                  {label}
                  {isTransfer && (
                    <span className="ml-2 inline-block rounded-full border border-border px-2 py-0.5 align-middle text-xs font-medium text-muted">
                      Traspaso
                    </span>
                  )}
                </p>
                <p className="mt-0.5 break-words text-sm text-muted">
                  {s.date} · {instrument || "Sin instrumento"} · {s.category || "Sin categoría"}
                </p>
              </div>
              <div className="shrink-0 text-left sm:text-right">
                <p className={`font-semibold tabular-nums ${withdrawal ? "text-destructive" : ""}`}>
                  {formatMoney(s.amount)} {s.currency}
                </p>
                {withdrawal && <p className="text-sm text-muted">Retiro</p>}
                {s.currency !== "MXN" && (
                  <p className="text-sm tabular-nums text-muted">
                    ≈ {formatMoney(s.amount_mxn)} MXN{s.exchange_rate ? ` (TC ${s.exchange_rate})` : ""}
                  </p>
                )}
              </div>
              {confirming ? (
                <div role="group" aria-label={`Confirmar eliminación de ${label}`} className="flex shrink-0 flex-wrap items-center gap-2">
                  <span className="text-sm">
                    {isTransfer ? "¿Eliminar este traspaso? Se eliminarán ambas partes (salida y entrada)." : "¿Eliminar este ahorro?"}
                  </span>
                  <button type="button" disabled={remove.isPending} onClick={() => remove.mutate(s.id)} className={dangerButton}>
                    {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                    Sí, eliminar
                  </button>
                  <button type="button" disabled={remove.isPending} onClick={() => setConfirmId(null)} className={secondaryButton}>
                    Cancelar
                  </button>
                </div>
              ) : (
                <div className="flex shrink-0 gap-2">
                  {!isTransfer && (
                    <button type="button" onClick={() => onEdit(s)} aria-label={`Editar ${label}`} className={secondaryButton}>
                      <Pencil className="h-4 w-4" aria-hidden="true" />
                      Editar
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      remove.reset();
                      setConfirmId(s.id);
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
