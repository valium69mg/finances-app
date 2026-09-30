import { useId } from "react";
import type { FutureExpenses } from "../../api/dashboard";
import { formatMoney, percentOf } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

/**
 * Expenses known in advance (yearly bills): due date, target, saved so far with a progress bar and the amount
 * to put aside each cycle to have each one on time, plus a total row.
 */
export function FutureExpensesCard({ future }: { future: FutureExpenses }) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <h3 id={titleId} className="font-semibold">
        Gastos futuros
      </h3>
      {future.items.length === 0 ? (
        <p className="mt-1 text-sm text-muted">
          Aún no hay gastos futuros. Agrega un pago recurrente anual (predial, seguro) con su monto y vencimiento para planear su ahorro.
        </p>
      ) : (
        <>
          <ul className="mt-3 space-y-4">
            {future.items.map((f) => {
              const pct = percentOf(f.saved, f.target);
              return (
                <li key={`${f.name}-${f.due_date}`}>
                  <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                    <span className="min-w-0 break-words font-medium">{f.name}</span>
                    <span className="text-sm text-muted">Vence el {dateLabel(f.due_date)}</span>
                  </div>
                  <div
                    role="progressbar"
                    aria-label={`Ahorro para ${f.name}`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={Math.round(pct)}
                    className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border"
                  >
                    <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
                  </div>
                  <p className="mt-1 text-sm tabular-nums">
                    {formatMoney(f.saved)} de {formatMoney(f.target)} ({Math.round(pct)}%)
                  </p>
                  <p className="text-sm tabular-nums text-muted">
                    {/^0(\.0+)?$/.test(f.remaining) ? "Meta cubierta." : `Aparta ${formatMoney(f.suggested_monthly)} al mes`}
                  </p>
                </li>
              );
            })}
          </ul>
          <dl className="mt-4 grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 border-t border-border pt-3 text-sm tabular-nums">
            <dt className="font-medium">Total</dt>
            <dd className="text-right font-semibold">{formatMoney(future.target)}</dd>
            <dt className="text-muted">Ahorrado</dt>
            <dd className="text-right">{formatMoney(future.saved)}</dd>
            <dt className="text-muted">Sugerido al mes</dt>
            <dd className="text-right">{formatMoney(future.suggested_monthly)}</dd>
          </dl>
        </>
      )}
    </section>
  );
}
