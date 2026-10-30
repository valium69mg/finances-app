import { ChevronRight } from "lucide-react";
import type { FutureExpenseItem } from "../../api/futureExpenses";
import { formatMoney, percentOf } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";
import { rowButton } from "../settings/ui";
import { useFutureBackend } from "./backend";
import { isEmptyBalance } from "./form";

/** Progress bar of the saved amount against the target. */
export function ItemProgress({ item }: { item: FutureExpenseItem }) {
  const pct = percentOf(item.saved, item.target_amount);
  return (
    <div>
      <div
        role="progressbar"
        aria-label={`Ahorro para ${item.name}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(pct)}
        className="h-2.5 w-full overflow-hidden rounded-full bg-border"
      >
        <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-1 text-sm tabular-nums">
        {formatMoney(item.saved)} de {formatMoney(item.target_amount)} ({Math.round(pct)}%)
      </p>
    </div>
  );
}

/** "Meta cubierta." or what to put aside each cycle. */
export function suggestionText(item: FutureExpenseItem): string {
  return isEmptyBalance(item.remaining)
    ? "Meta cubierta."
    : `Aparta ${formatMoney(item.suggested_monthly)} al mes (${item.cycles_left} ${item.cycles_left === 1 ? "ciclo" : "ciclos"} restantes)`;
}

function StatusChip({ item }: { item: FutureExpenseItem }) {
  const covered = isEmptyBalance(item.remaining);
  return (
    <span className={`inline-flex shrink-0 items-center rounded-full px-2.5 py-1 text-xs font-medium ${covered ? "bg-accent/15 text-foreground" : "bg-border/50 text-muted"}`}>
      {covered ? "Meta cubierta" : "En proceso"}
    </span>
  );
}

function ActiveSummary({ item, trailing }: { item: FutureExpenseItem; trailing?: boolean }) {
  return (
    <>
      <span className="min-w-0 flex-1 space-y-3">
        <span className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
          <span className="min-w-0 break-words font-medium">{item.name}</span>
          <StatusChip item={item} />
        </span>
        <ItemProgress item={item} />
        <span className="block text-sm text-muted">
          Vence el {dateLabel(item.due_date)}
          {!isEmptyBalance(item.remaining) && <span className="block tabular-nums">{suggestionText(item)}</span>}
        </span>
      </span>
      {trailing && <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />}
    </>
  );
}

interface ActiveProps {
  items: FutureExpenseItem[];
  /** Opens the detail with the item's actions (hers only; the read-only view has no detail). */
  onOpen: (item: FutureExpenseItem) => void;
}

/** Active future expenses as summary cards: name, status, progress, due date. A card opens its detail dialog. */
export function ActiveList({ items, onOpen }: ActiveProps) {
  const readOnly = useFutureBackend().readOnly === true;
  return (
    <ul aria-label="Gastos futuros activos" className="space-y-2">
      {items.map((f) => (
        <li key={f.id}>
          {readOnly ? (
            <div className="flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3">
              <ActiveSummary item={f} />
            </div>
          ) : (
            <button type="button" onClick={() => onOpen(f)} aria-label={`Ver ${f.name}`} className={`${rowButton} items-start`}>
              <ActiveSummary item={f} trailing />
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}

function PaidSummary({ item, trailing }: { item: FutureExpenseItem; trailing?: boolean }) {
  return (
    <>
      <span className="min-w-0 flex-1">
        <span className="block break-words font-medium">{item.name}</span>
        <span className="block text-sm text-muted">
          Pagado el {item.paid_at ? dateLabel(item.paid_at) : "—"} · {item.amount_paid ? formatMoney(item.amount_paid) : "—"} de {formatMoney(item.target_amount)} de meta
        </span>
      </span>
      {trailing && <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />}
    </>
  );
}

/** Paid items, most recent first: what was paid and when. A row opens a dialog whose only action is removing it from the history. */
export function PaidList({ items, onOpen }: ActiveProps) {
  const readOnly = useFutureBackend().readOnly === true;
  return (
    <ul aria-label="Gastos futuros pagados" className="space-y-2">
      {items.map((f) => (
        <li key={f.id}>
          {readOnly ? (
            <div className="flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3">
              <PaidSummary item={f} />
            </div>
          ) : (
            <button type="button" onClick={() => onOpen(f)} aria-label={`Ver ${f.name}`} className={rowButton}>
              <PaidSummary item={f} trailing />
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}
