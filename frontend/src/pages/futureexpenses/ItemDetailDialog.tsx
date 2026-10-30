import { useMutation } from "@tanstack/react-query";
import { ArrowRightLeft, CheckCircle2, Pencil, PiggyBank } from "lucide-react";
import type { FutureExpenseItem } from "../../api/futureExpenses";
import { DeleteAction } from "../../components/DeleteAction";
import { Modal } from "../../components/Modal";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { dateLabel } from "../taxfiling/labels";
import { useFutureBackend } from "./backend";
import { describeFutureError } from "./errors";
import { isEmptyBalance } from "./form";
import { ItemProgress, suggestionText } from "./ItemLists";
import { useInvalidateFuture } from "./useInvalidate";

interface Props {
  item: FutureExpenseItem;
  /** Free balance: the assign action only shows while there is something to assign. */
  freeBalance: string;
  onClose: () => void;
  onSaving: (item: FutureExpenseItem) => void;
  onAssign: (item: FutureExpenseItem) => void;
  onPay: (item: FutureExpenseItem) => void;
  onEdit: (item: FutureExpenseItem) => void;
  /** Called after the item was deleted so the page can show a notice. */
  onDeleted: (item: FutureExpenseItem) => void;
}

/**
 * The detail of one pending future expense and everything that can be done with it: add savings, assign the free
 * balance, mark it paid, edit it or delete it (after a confirmation). Each action hands over to its own dialog.
 */
export function ItemDetailDialog({ item, freeBalance, onClose, onSaving, onAssign, onPay, onEdit, onDeleted }: Props) {
  const backend = useFutureBackend();
  const invalidate = useInvalidateFuture();
  const remove = useMutation({
    mutationFn: () => backend.remove(item.id),
    onSuccess: async () => {
      onDeleted(item);
      await invalidate();
    },
  });

  return (
    <Modal title={item.name} onClose={onClose}>
      <div className="space-y-5">
        <ItemProgress item={item} />
        <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm tabular-nums">
          <dt className="text-muted">Vence el</dt>
          <dd className="text-right">{dateLabel(item.due_date)}</dd>
          <dt className="text-muted">Falta</dt>
          <dd className="text-right">{formatMoney(item.remaining)}</dd>
        </dl>
        <p className="text-sm tabular-nums text-muted">{suggestionText(item)}</p>

        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => onSaving(item)} aria-label={`Agregar ahorro a ${item.name}`} className={secondaryButton}>
            <PiggyBank className="h-4 w-4" aria-hidden="true" />
            Agregar ahorro
          </button>
          {!isEmptyBalance(freeBalance) && (
            <button type="button" onClick={() => onAssign(item)} aria-label={`Asignar saldo libre a ${item.name}`} className={secondaryButton}>
              <ArrowRightLeft className="h-4 w-4" aria-hidden="true" />
              Asignar saldo libre
            </button>
          )}
          <button type="button" onClick={() => onPay(item)} aria-label={`Marcar pagado ${item.name}`} className={secondaryButton}>
            <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
            Marcar pagado
          </button>
          <button type="button" onClick={() => onEdit(item)} aria-label={`Editar ${item.name}`} className={secondaryButton}>
            <Pencil className="h-4 w-4" aria-hidden="true" />
            Editar
          </button>
        </div>

        {remove.isError && <ErrorBanner>{describeFutureError(remove.error)}</ErrorBanner>}
        <div className="border-t border-border pt-5">
          <DeleteAction
            name={item.name}
            question="¿Eliminar este gasto futuro? Su ahorro no se borra: vuelve al saldo libre."
            pending={remove.isPending}
            onStart={() => remove.reset()}
            onConfirm={() => remove.mutate()}
          />
        </div>
      </div>
    </Modal>
  );
}

/** A paid item: what was paid and the one action left, removing it from the history (the registered expense stays). */
export function PaidDetailDialog({ item, onClose, onDeleted }: { item: FutureExpenseItem; onClose: () => void; onDeleted: (item: FutureExpenseItem) => void }) {
  const backend = useFutureBackend();
  const invalidate = useInvalidateFuture();
  const remove = useMutation({
    mutationFn: () => backend.remove(item.id),
    onSuccess: async () => {
      onDeleted(item);
      await invalidate();
    },
  });
  return (
    <Modal title={item.name} onClose={onClose}>
      <div className="space-y-5">
        <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm tabular-nums">
          <dt className="text-muted">Pagado el</dt>
          <dd className="text-right">{item.paid_at ? dateLabel(item.paid_at) : "—"}</dd>
          <dt className="text-muted">Monto pagado</dt>
          <dd className="text-right">{item.amount_paid ? formatMoney(item.amount_paid) : "—"}</dd>
          <dt className="text-muted">Meta</dt>
          <dd className="text-right">{formatMoney(item.target_amount)}</dd>
        </dl>
        {remove.isError && <ErrorBanner>{describeFutureError(remove.error)}</ErrorBanner>}
        <DeleteAction
          name={item.name}
          question="¿Quitarlo del historial? El gasto registrado se conserva."
          pending={remove.isPending}
          onStart={() => remove.reset()}
          onConfirm={() => remove.mutate()}
        />
      </div>
    </Modal>
  );
}
