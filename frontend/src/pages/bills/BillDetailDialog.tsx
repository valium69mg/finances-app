import { useState } from "react";
import { Loader2, Pencil, Power, RotateCcw, SkipForward } from "lucide-react";
import type { Bill } from "../../api/bills";
import { Modal } from "../../components/Modal";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { dateLabel } from "../taxfiling/labels";
import { DueBadge } from "./DueBadge";
import { describeBillsError } from "./errors";
import { HistoryPanel } from "./HistoryPanel";
import { RECURRENCE_LABEL, leadLabel } from "./labels";

/** The mutations of a bill (owned by the list so its card can reactivate too). */
export interface BillActions {
  skip: (bill: Bill) => void;
  deactivate: (bill: Bill) => void;
  reactivate: (bill: Bill) => void;
  busy: boolean;
  /** Last failure of any of them, if any. */
  error: unknown;
  /** Forget the last failure (a new action starts). */
  reset: () => void;
}

interface Props {
  bill: Bill;
  actions: BillActions;
  onClose: () => void;
  onEdit: (bill: Bill) => void;
}

type Confirm = "skip" | "deactivate";

/**
 * Everything about one recurring payment that is not "pay it": its details, edit, skip this due date, deactivate or
 * reactivate (with confirmations) and its history. "Pagar" stays on the card because it is the common action.
 */
export function BillDetailDialog({ bill, actions, onClose, onEdit }: Props) {
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const { busy } = actions;

  function ask(kind: Confirm) {
    actions.reset();
    setConfirm(kind);
  }

  return (
    <Modal title={bill.name} onClose={onClose}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-2">
          {!bill.active && <span className="inline-block whitespace-nowrap rounded-full border border-border bg-background px-2.5 py-0.5 text-xs font-medium">Desactivado</span>}
          <DueBadge bill={bill} />
        </div>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted">Monto</dt>
          <dd className="text-right font-medium tabular-nums">{bill.amount === null ? "Variable" : `${formatMoney(bill.amount)} ${bill.currency}`}</dd>
          <dt className="text-muted">Recurrencia</dt>
          <dd className="text-right">{RECURRENCE_LABEL[bill.recurrence]}</dd>
          <dt className="text-muted">Categoría</dt>
          <dd className="min-w-0 break-words text-right">{bill.category}</dd>
          {bill.active && (
            <>
              <dt className="text-muted">Próximo vencimiento</dt>
              <dd className="text-right">{dateLabel(bill.next_due_date)}</dd>
            </>
          )}
          <dt className="text-muted">Aviso</dt>
          <dd className="text-right">{leadLabel(bill.reminder_lead_days)}</dd>
        </dl>
        {bill.notes && <p className="break-words text-sm text-muted">{bill.notes}</p>}

        {confirm ? (
          <div role="group" aria-label={`Confirmar ${confirm === "skip" ? "omitir" : "desactivar"} ${bill.name}`} className="flex flex-wrap items-center gap-2">
            <span className="text-sm">{confirm === "skip" ? "¿Omitir este vencimiento? No se registra ningún gasto." : "¿Desactivar este pago recurrente? Su historial se conserva."}</span>
            <button type="button" disabled={busy} onClick={() => (confirm === "skip" ? actions.skip(bill) : actions.deactivate(bill))} className={dangerButton}>
              {busy && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
              {confirm === "skip" ? "Sí, omitir" : "Sí, desactivar"}
            </button>
            <button type="button" disabled={busy} onClick={() => setConfirm(null)} className={secondaryButton}>
              Cancelar
            </button>
          </div>
        ) : (
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => onEdit(bill)} aria-label={`Editar ${bill.name}`} className={secondaryButton}>
              <Pencil className="h-4 w-4" aria-hidden="true" />
              Editar
            </button>
            {bill.active ? (
              <>
                <button type="button" onClick={() => ask("skip")} aria-label={`Omitir ${bill.name}`} className={secondaryButton}>
                  <SkipForward className="h-4 w-4" aria-hidden="true" />
                  Omitir
                </button>
                <button type="button" onClick={() => ask("deactivate")} aria-label={`Desactivar ${bill.name}`} className={secondaryButton}>
                  <Power className="h-4 w-4" aria-hidden="true" />
                  Desactivar
                </button>
              </>
            ) : (
              <button type="button" disabled={busy} onClick={() => actions.reactivate(bill)} aria-label={`Reactivar ${bill.name}`} className={secondaryButton}>
                <RotateCcw className="h-4 w-4" aria-hidden="true" />
                Reactivar
              </button>
            )}
          </div>
        )}
        {actions.error != null && <ErrorBanner>{describeBillsError(actions.error)}</ErrorBanner>}

        <section aria-label="Historial" className="space-y-2 border-t border-border pt-5">
          <h3 className="text-base font-semibold tracking-tight">Historial</h3>
          <HistoryPanel billId={bill.id} name={bill.name} />
        </section>
      </div>
    </Modal>
  );
}
