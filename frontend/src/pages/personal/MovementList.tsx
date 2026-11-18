import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Loader2 } from "lucide-react";
import { deletePersonalMovement, listPersonalMovements, personalKeys, type PersonalCategory, type PersonalMovement } from "../../api/personal";
import { AssignmentDialog } from "../../components/AssignmentDialog";
import { CoveredBadge } from "../../components/CoveredBadge";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND } from "../../lib/tones";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, rowButton, secondaryButton } from "../settings/ui";
import type { PersonalPageConfig } from "./config";
import { describePersonalError } from "./errors";
import { usePersonalScope } from "./scope";
import { isNegative, sumMoney } from "./totals";

interface Props {
  config: PersonalPageConfig;
  /** YYYY-MM cycle label; empty asks the server for the current cycle. */
  month: string;
  /** Category the list is narrowed to, if any. */
  category?: PersonalCategory;
  /** False while the categories load: the filter is only known once they have, so the list waits for it. */
  ready?: boolean;
  /** Opens the edit dialog (hers only; the read-only Hogar view renders plain rows). */
  onEdit: (movement: PersonalMovement) => void;
}

export function MovementList({ config, month, category, ready = true, onEdit }: Props) {
  const { userId, readOnly } = usePersonalScope();
  const qc = useQueryClient();
  const [assignment, setAssignment] = useState<PersonalMovement | null>(null);
  const removeAssignment = useMutation({
    mutationFn: (id: number) => deletePersonalMovement(id),
    onSuccess: async () => {
      setAssignment(null);
      await qc.invalidateQueries({ queryKey: personalKeys.all });
    },
  });
  const list = useQuery({
    queryKey: personalKeys.movements(config.kind, month, category?.id ?? 0, userId ?? undefined),
    queryFn: () => listPersonalMovements({ month, kind: config.kind, categoryId: category?.id, userId: userId ?? undefined }),
    enabled: ready,
    retry: false,
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
  // An assignment is two linked rows; the list shows only the one that feeds the item (the positive leg).
  const shown = new Set(list.data.filter((m) => m.transfer_id && !isNegative(m.amount)).map((m) => m.transfer_id));
  const rows = list.data.filter((m) => !(m.transfer_id && isNegative(m.amount) && shown.has(m.transfer_id)));
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted">
        {config.totalLabel}: <strong data-testid="personal-total" className={`text-base tabular-nums ${isNegative(total) ? "text-destructive" : "text-foreground"}`}>{formatMoney(total)} MXN</strong>
      </p>
      <ul className="space-y-2">
        {rows.map((m) => {
          const isAssignment = Boolean(m.transfer_id);
          const label = m.description || "Sin descripción";
          const withdrawal = config.kind === "Ahorro" && isNegative(m.amount) && !isAssignment;
          const body = (
            <>
              <span className="min-w-0 flex-1">
                <span className="block break-words font-medium">
                  {label}
                  {isAssignment && (
                    <span className="ml-2 inline-block rounded-full border border-border px-2 py-0.5 align-middle text-xs font-medium text-muted">Asignación</span>
                  )}
                </span>
                <span className="mt-0.5 block break-words text-sm text-muted">
                  {m.date} · {m.category}
                  {config.kind === "Ahorro" && !isAssignment ? ` · ${withdrawal ? "Retiro" : "Aportación"}` : ""}
                </span>
                {config.kind === "Gasto" && <CoveredBadge covered={m.covered_by_savings} amount={m.amount} />}
              </span>
              <span className={`shrink-0 font-semibold tabular-nums ${withdrawal ? "text-destructive" : ""}`}>{formatMoney(m.amount)} MXN</span>
              {!readOnly && <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />}
            </>
          );
          return (
            <li key={m.id}>
              {readOnly ? (
                <div className="flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3">{body}</div>
              ) : (
                <button
                  type="button"
                  onClick={() => (isAssignment ? setAssignment(m) : onEdit(m))}
                  aria-label={`${isAssignment ? "Ver" : "Editar"} ${label}`} 
                  className={rowButton}
                >
                  {body}
                </button>
              )}
            </li>
          );
        })}
      </ul>
      {assignment && (
        <AssignmentDialog
          label={assignment.description || "Asignación"}
          date={assignment.date}
          amount={assignment.amount}
          pending={removeAssignment.isPending}
          error={removeAssignment.isError ? describePersonalError(removeAssignment.error, config.noun) : null}
          onStart={() => removeAssignment.reset()}
          onConfirm={() => removeAssignment.mutate(assignment.id)}
          onClose={() => setAssignment(null)}
        />
      )}
    </div>
  );
}
