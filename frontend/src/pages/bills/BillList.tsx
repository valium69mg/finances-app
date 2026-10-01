import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, ChevronDown, ChevronUp, Loader2, Pencil, Power, RotateCcw, SkipForward, Wallet } from "lucide-react";
import { billsKeys, deactivateBill, listBills, skipBill, updateBill, type Bill } from "../../api/bills";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { DueBadge } from "./DueBadge";
import { describeBillsError } from "./errors";
import { HistoryPanel } from "./HistoryPanel";
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

type Confirm = { id: number; kind: "skip" | "deactivate" };

/** The bills, sorted by next due date by the API, with their actions. */
export function BillList({ includeInactive, editingId, onPay, onEdit, onNotice, onChanged }: Props) {
  const qc = useQueryClient();
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [expanded, setExpanded] = useState<number | null>(null);
  const list = useQuery({ queryKey: billsKeys.list(includeInactive), queryFn: () => listBills(includeInactive), retry: false });

  const refresh = () => qc.invalidateQueries({ queryKey: billsKeys.all });
  const skip = useMutation({
    mutationFn: (bill: Bill) => skipBill(bill.id),
    onSuccess: async (next, bill) => {
      setConfirm(null);
      onChanged(bill.id);
      onNotice(`Se omitió el vencimiento de ${bill.name}. No se registró ningún gasto. Próximo vencimiento: ${dateLabel(next.next_due_date)}.`);
      await refresh();
    },
  });
  const deactivate = useMutation({
    mutationFn: (bill: Bill) => deactivateBill(bill.id),
    onSuccess: async (_, bill) => {
      setConfirm(null);
      onChanged(bill.id);
      onNotice(`${bill.name} se desactivó. Su historial y sus gastos se conservan.`);
      await refresh();
    },
  });
  const reactivate = useMutation({
    mutationFn: (bill: Bill) => updateBill(bill.id, toBillInput(draftOf(bill), true)),
    onSuccess: async (_, bill) => {
      onNotice(`${bill.name} se reactivó.`);
      await refresh();
    },
  });
  const busy = skip.isPending || deactivate.isPending || reactivate.isPending;
  const failure = skip.error ?? deactivate.error ?? reactivate.error;

  function ask(bill: Bill, kind: Confirm["kind"]) {
    skip.reset();
    deactivate.reset();
    reactivate.reset();
    setConfirm({ id: bill.id, kind });
  }

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

  return (
    <div className="space-y-3">
      {failure && <ErrorBanner>{describeBillsError(failure)}</ErrorBanner>}
      <ul aria-label="Pagos recurrentes" className="divide-y divide-border rounded-xl border border-border">
        {list.data.map((b) => {
          const confirming = confirm?.id === b.id ? confirm.kind : null;
          const open = expanded === b.id;
          const panelId = `bill-history-${b.id}`;
          return (
            <li key={b.id} className={editingId === b.id ? "bg-primary/5" : undefined}>
              <div className="flex flex-col gap-3 p-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="break-words font-medium">{b.name}</p>
                    {!b.active && <span className="inline-block whitespace-nowrap rounded-full border border-border bg-background px-2.5 py-0.5 text-xs font-medium">Desactivado</span>}
                    <DueBadge bill={b} />
                  </div>
                  <p className="mt-0.5 break-words text-sm text-muted">
                    {RECURRENCE_LABEL[b.recurrence]} · {b.category} · {leadLabel(b.reminder_lead_days)}
                  </p>
                  {b.active && (
                    <p className="mt-0.5 text-sm">
                      Próximo vencimiento: <span className="font-medium">{dateLabel(b.next_due_date)}</span>
                    </p>
                  )}
                  {b.notes && <p className="mt-0.5 break-words text-sm text-muted">{b.notes}</p>}
                </div>
                <p className="shrink-0 font-semibold tabular-nums lg:text-right">{b.amount === null ? <span className="font-medium text-muted">Monto variable</span> : `${formatMoney(b.amount)} ${b.currency}`}</p>

                {confirming ? (
                  <div role="group" aria-label={`Confirmar ${confirming === "skip" ? "omitir" : "desactivar"} ${b.name}`} className="flex shrink-0 flex-wrap items-center gap-2">
                    <span className="text-sm">
                      {confirming === "skip" ? "¿Omitir este vencimiento? No se registra ningún gasto." : "¿Desactivar este pago recurrente? Su historial se conserva."}
                    </span>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => (confirming === "skip" ? skip.mutate(b) : deactivate.mutate(b))}
                      className={dangerButton}
                    >
                      {busy && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                      {confirming === "skip" ? "Sí, omitir" : "Sí, desactivar"}
                    </button>
                    <button type="button" disabled={busy} onClick={() => setConfirm(null)} className={secondaryButton}>
                      Cancelar
                    </button>
                  </div>
                ) : (
                  <div className="flex shrink-0 flex-wrap gap-2">
                    {b.active ? (
                      <>
                        <button type="button" onClick={() => onPay(b)} aria-label={`Pagar ${b.name}`} className={secondaryButton}>
                          <Wallet className="h-4 w-4" aria-hidden="true" />
                          Pagar
                        </button>
                        <button type="button" onClick={() => ask(b, "skip")} aria-label={`Omitir ${b.name}`} className={secondaryButton}>
                          <SkipForward className="h-4 w-4" aria-hidden="true" />
                          Omitir
                        </button>
                      </>
                    ) : (
                      <button type="button" disabled={busy} onClick={() => reactivate.mutate(b)} aria-label={`Reactivar ${b.name}`} className={secondaryButton}>
                        <RotateCcw className="h-4 w-4" aria-hidden="true" />
                        Reactivar
                      </button>
                    )}
                    <button type="button" onClick={() => onEdit(b)} aria-label={`Editar ${b.name}`} className={secondaryButton}>
                      <Pencil className="h-4 w-4" aria-hidden="true" />
                      Editar
                    </button>
                    {b.active && (
                      <button type="button" onClick={() => ask(b, "deactivate")} aria-label={`Desactivar ${b.name}`} className={secondaryButton}>
                        <Power className="h-4 w-4" aria-hidden="true" />
                        Desactivar
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => setExpanded(open ? null : b.id)}
                      aria-expanded={open}
                      aria-controls={panelId}
                      aria-label={`${open ? "Ocultar" : "Ver"} historial de ${b.name}`}
                      className={secondaryButton}
                    >
                      {open ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
                      Historial
                    </button>
                  </div>
                )}
              </div>
              {open && (
                <div id={panelId} className="border-t border-border bg-background/50 p-4">
                  <HistoryPanel billId={b.id} name={b.name} />
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
