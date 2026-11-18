import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CalendarClock, Loader2 } from "lucide-react";
import { Link } from "react-router-dom";
import { billsKeys, listBills, type Bill } from "../api/bills";
import { DueBadge } from "../pages/bills/DueBadge";
import { describeBillsError } from "../pages/bills/errors";
import { PayBillForm } from "../pages/bills/PayBillForm";
import { paidNotice } from "../pages/bills/paidNotice";
import { formatMoney } from "../pages/expenses/money";
import { ErrorBanner, secondaryButton, tapLink } from "../pages/settings/ui";
import { dateLabel } from "../pages/taxfiling/labels";

interface Props {
  /** Expense category names the payment can be filed under. */
  categories: string[];
  onClose: () => void;
  /** Called with the confirmation text once a payment is registered; the dialog is closed right after. */
  onPaid: (message: string) => void;
}

/** Bills with a pending occurrence, soonest (or most overdue) first. */
export const pendingBills = (bills: Bill[]): Bill[] =>
  bills
    .filter((b) => b.active && b.pending_occurrence !== null)
    .sort((a, b) => (a.next_due_date < b.next_due_date ? -1 : a.next_due_date > b.next_due_date ? 1 : a.id - b.id));

/**
 * "Recurrente" tab of the dock's quick dialog: pick a pending bill and register its payment with the same form, request
 * and error mapping as the "Pagar" dialog of Pagos recurrentes.
 */
export function QuickBills({ categories, onClose, onPaid }: Props) {
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const list = useQuery({ queryKey: billsKeys.list(false), queryFn: () => listBills(false), retry: false });

  if (list.isPending)
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando pagos recurrentes…
      </p>
    );
  if (list.isError)
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeBillsError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );

  const bills = pendingBills(list.data);
  if (bills.length === 0)
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted">No tienes pagos recurrentes pendientes.</p>
        <Link to="/pagos-recurrentes" onClick={onClose} className={`${tapLink} text-sm`}>
          Ir a Pagos recurrentes
        </Link>
      </div>
    );

  const selected = bills.find((b) => b.id === selectedId) ?? null;
  return (
    <div className="space-y-5">
      <fieldset>
        <legend className="mb-2 text-sm font-medium">Elige el pago</legend>
        <ul className="max-h-60 space-y-2 overflow-y-auto pr-1">
          {bills.map((b) => (
            <li key={b.id}>
              <label className="flex min-h-14 cursor-pointer items-start gap-3 rounded-xl border border-border p-3 focus-within:ring-2 focus-within:ring-primary has-[:checked]:border-primary has-[:checked]:bg-primary/5">
                <input type="radio" name="quick-bill" checked={b.id === selectedId} onChange={() => setSelectedId(b.id)} className="mt-1 h-4 w-4 shrink-0 accent-primary" />
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-baseline justify-between gap-x-3">
                    <span className="min-w-0 break-words font-medium">{b.name}</span>
                    <span className="shrink-0 text-sm tabular-nums">{b.amount === null ? "Variable" : `${formatMoney(b.amount)} ${b.currency}`}</span>
                  </span>
                  <span className="mt-1 flex flex-wrap items-center gap-2 text-sm text-muted">
                    <CalendarClock className="h-4 w-4 shrink-0" aria-hidden="true" />
                    <span>{dateLabel(b.next_due_date)}</span>
                    <DueBadge bill={b} />
                  </span>
                </span>
              </label>
            </li>
          ))}
        </ul>
      </fieldset>
      {selected ? (
        <PayBillForm key={selected.id} bill={selected} categories={categories} onCancel={onClose} onPaid={(result) => onPaid(paidNotice(result))} />
      ) : (
        <p className="text-sm text-muted">Elige un pago para registrarlo.</p>
      )}
    </div>
  );
}
