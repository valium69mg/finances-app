import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, ChevronRight, Loader2, RotateCcw, Wallet } from "lucide-react";
import { billsKeys, deactivateBill, listBills, skipBill, updateBill, type Bill } from "../../api/bills";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { BillDetailDialog, type BillActions } from "./BillDetailDialog";
import { DueBadge } from "./DueBadge";
import { describeBillsError } from "./errors";
import { RECURRENCE_LABEL, leadLabel } from "./labels";
import { toBillInput, draftOf } from "./form";
import { EmptyNote } from "../../components/EmptyNote";

interface Props {
  includeInactive: boolean;
  editingId: number | null;
  onPay: (bill: Bill) => void;
  onEdit: (bill: Bill) => void;
  /** Called with a confirmation message after skipping, deactivating or reactivating. */
  onNotice: (message: string) => void;
  /** Called after a bill was skipped or deactivated so the page can drop a stale edit (its due date moved). */
  onChanged: (id: number) => void;
}

/** The bills, sorted by next due date by the API, with their actions. */
export function BillList({ includeInactive, editingId, onPay, onEdit, onNotice, onChanged }: Props) {
  const qc = useQueryClient();
  const [detailId, setDetailId] = useState<number | null>(null);
  const list = useQuery({ queryKey: billsKeys.list(includeInactive), queryFn: () => listBills(includeInactive), retry: false });

  const refresh = () => qc.invalidateQueries({ queryKey: billsKeys.all });
  const skip = useMutation({
    mutationFn: (bill: Bill) => skipBill(bill.id),
    onSuccess: async (next, bill) => {
      setDetailId(null);
      onChanged(bill.id);
      onNotice(`Se omitió el vencimiento de ${bill.name}. No se registró ningún gasto. Próximo vencimiento: ${dateLabel(next.next_due_date)}.`);
      await refresh();
    },
  });
  const deactivate = useMutation({
    mutationFn: (bill: Bill) => deactivateBill(bill.id),
    onSuccess: async (_, bill) => {
      setDetailId(null);
      onChanged(bill.id);
      onNotice(`${bill.name} se desactivó. Su historial y sus gastos se conservan.`);
      await refresh();
    },
  });
  const reactivate = useMutation({
    mutationFn: (bill: Bill) => updateBill(bill.id, toBillInput(draftOf(bill), true)),
    onSuccess: async (_, bill) => {
      setDetailId(null);
      onNotice(`${bill.name} se reactivó.`);
      await refresh();
    },
  });
  const busy = skip.isPending || deactivate.isPending || reactivate.isPending;
  const failure = skip.error ?? deactivate.error ?? reactivate.error;

  const actions: BillActions = {
    skip: (b) => skip.mutate(b),
    deactivate: (b) => deactivate.mutate(b),
    reactivate: (b) => reactivate.mutate(b),
    busy,
    error: failure,
    reset: () => {
      skip.reset();
      deactivate.reset();
      reactivate.reset();
    },
  };

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando pagos recurrentes…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeBillsError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    return <EmptyNote icon={CalendarClock}>Aún no tienes pagos recurrentes. Agrega el primero con el formulario de arriba.</EmptyNote>;
  }

  const detail = list.data.find((b) => b.id === detailId) ?? null;
  return (
    <div className="space-y-3">
      {/* A failure belongs to the dialog while it is open; the card only shows the reactivation one. */}
      {failure && !detail && <ErrorBanner>{describeBillsError(failure)}</ErrorBanner>}
      <ul aria-label="Pagos recurrentes" className="space-y-2">
        {list.data.map((b) => (
          <li key={b.id} className={`flex flex-col rounded-xl border border-border bg-surface sm:flex-row sm:items-center ${editingId === b.id ? "bg-primary/5" : ""}`}>
            <button
              type="button"
              onClick={() => {
                actions.reset();
                setDetailId(b.id);
              }}
              aria-label={`Ver ${b.name}`}
              className="focus-ring flex min-h-11 min-w-0 flex-1 items-center gap-3 rounded-xl px-4 py-3 text-left transition-colors duration-200 hover:bg-primary/5"
            >
              <span className="min-w-0 flex-1">
                <span className="flex flex-wrap items-center gap-2">
                  <span className="break-words font-medium">{b.name}</span>
                  {!b.active && <span className="inline-block whitespace-nowrap rounded-full border border-border bg-background px-2.5 py-0.5 text-xs font-medium">Desactivado</span>}
                  <DueBadge bill={b} />
                </span>
                <span className="mt-0.5 block break-words text-sm text-muted">
                  {RECURRENCE_LABEL[b.recurrence]} · {b.category} · {leadLabel(b.reminder_lead_days)}
                </span>
                {b.active && (
                  <span className="mt-0.5 block text-sm">
                    Próximo vencimiento: <span className="font-medium">{dateLabel(b.next_due_date)}</span>
                  </span>
                )}
                {b.notes && <span className="mt-0.5 block break-words text-sm text-muted">{b.notes}</span>}
                <span className="mt-1 block font-semibold tabular-nums">
                  {b.amount === null ? <span className="font-medium text-muted">Monto variable</span> : `${formatMoney(b.amount)} ${b.currency}`}
                </span>
              </span>
              <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
            </button>
            <div className="flex shrink-0 items-center px-4 pb-3 sm:px-0 sm:pb-0 sm:pr-3 [&>button]:w-full sm:[&>button]:w-auto">
              {b.active ? (
                <button type="button" onClick={() => onPay(b)} aria-label={`Pagar ${b.name}`} className={secondaryButton}>
                  <Wallet className="h-4 w-4" aria-hidden="true" />
                  Pagar
                </button>
              ) : (
                <button type="button" disabled={busy} onClick={() => reactivate.mutate(b)} aria-label={`Reactivar ${b.name}`} className={secondaryButton}>
                  <RotateCcw className="h-4 w-4" aria-hidden="true" />
                  Reactivar
                </button>
              )}
            </div>
          </li>
        ))}
      </ul>
      {detail && (
        <BillDetailDialog
          bill={detail}
          actions={actions}
          onClose={() => setDetailId(null)}
          onEdit={(b) => {
            setDetailId(null);
            onEdit(b);
          }}
        />
      )}
    </div>
  );
}
