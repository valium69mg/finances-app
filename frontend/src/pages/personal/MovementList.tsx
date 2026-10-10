import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Trash2 } from "lucide-react";
import { deletePersonalMovement, listPersonalMovements, personalKeys, type PersonalCategory, type PersonalMovement } from "../../api/personal";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND } from "../../lib/tones";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import type { PersonalPageConfig } from "./config";
import { describePersonalError } from "./errors";
import { isNegative, sumMoney } from "./totals";

interface Props {
  config: PersonalPageConfig;
  /** YYYY-MM cycle label; empty asks the server for the current cycle. */
  month: string;
  /** Category the list is narrowed to, if any. */
  category?: PersonalCategory;
  /** False while the categories load: the filter is only known once they have, so the list waits for it. */
  ready?: boolean;
  onEdit: (movement: PersonalMovement) => void;
  onDeleted: (id: number) => void;
}

export function MovementList({ config, month, category, ready = true, onEdit, onDeleted }: Props) {
  const qc = useQueryClient();
  const [confirmId, setConfirmId] = useState<number | null>(null);
  const list = useQuery({
    queryKey: personalKeys.movements(config.kind, month, category?.id ?? 0),
    queryFn: () => listPersonalMovements({ month, kind: config.kind, categoryId: category?.id }),
    enabled: ready,
    retry: false,
  });
  const remove = useMutation({
    mutationFn: (id: number) => deletePersonalMovement(id),
    onSuccess: (_, id) => {
      setConfirmId(null);
      onDeleted(id);
      return qc.invalidateQueries({ queryKey: personalKeys.all });
    },
  });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        {config.loading}
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describePersonalError(list.error, config.noun)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    return (
      <EmptyNote icon={KIND[config.kind].icon}>{category ? `No hay movimientos de la categoría ${category.name} en este periodo.` : config.empty}</EmptyNote>
    );
  }

  const total = sumMoney(list.data.map((m) => m.amount));
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted">
        {config.totalLabel}: <strong data-testid="personal-total" className={`text-base tabular-nums ${isNegative(total) ? "text-destructive" : "text-foreground"}`}>{formatMoney(total)} MXN</strong>
      </p>
      {remove.isError && <ErrorBanner>{describePersonalError(remove.error, config.noun)}</ErrorBanner>}
      <ul className="divide-y divide-border rounded-xl border border-border">
        {list.data.map((m) => {
          const label = m.description || "Sin descripción";
          const confirming = confirmId === m.id;
          const withdrawal = config.kind === "Ahorro" && isNegative(m.amount);
          return (
            <li key={m.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0 flex-1">
                <p className="break-words font-medium">{label}</p>
                <p className="mt-0.5 break-words text-sm text-muted">
                  {m.date} · {m.category}
                  {config.kind === "Ahorro" ? ` · ${withdrawal ? "Retiro" : "Aportación"}` : ""}
                </p>
              </div>
              <p className={`shrink-0 text-left font-semibold tabular-nums sm:text-right ${withdrawal ? "text-destructive" : ""}`}>{formatMoney(m.amount)} MXN</p>
              {confirming ? (
                <div role="group" aria-label={`Confirmar eliminación de ${label}`} className="flex shrink-0 flex-wrap items-center gap-2">
                  <span className="text-sm">{config.deleteQuestion}</span>
                  <button type="button" disabled={remove.isPending} onClick={() => remove.mutate(m.id)} className={dangerButton}>
                    {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                    Sí, eliminar
                  </button>
                  <button type="button" disabled={remove.isPending} onClick={() => setConfirmId(null)} className={secondaryButton}>
                    Cancelar
                  </button>
                </div>
              ) : (
                <div className="flex shrink-0 gap-2">
                  <button type="button" onClick={() => onEdit(m)} aria-label={`Editar ${label}`} className={secondaryButton}>
                    <Pencil className="h-4 w-4" aria-hidden="true" />
                    Editar
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      remove.reset();
                      setConfirmId(m.id);
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
