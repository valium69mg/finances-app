import { Target } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router-dom";
import type { FutureExpenses } from "../../api/dashboard";
import { CardTitle } from "../../components/CardTitle";
import { EmptyNote } from "../../components/EmptyNote";
import { formatMoney, percentOf } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

const isZero = (v: string) => /^0(\.0+)?$/.test(v);

/**
 * The active future expenses the owner registered (Configuración > Gastos futuros): due date, target, saved so
 * far with a progress bar and the amount to put aside each cycle to have each one on time, a total row and the
 * free balance (savings in Gastos futuros not assigned to any item yet).
 */
export function FutureExpensesCard({ future }: { future: FutureExpenses }) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <CardTitle id={titleId} icon={Target} tone="saving">
        Gastos futuros
      </CardTitle>
      {future.items.length === 0 ? (
        <EmptyNote icon={Target}>
          Aún no hay gastos futuros. Regístralos en Configuración, con su monto y fecha de vencimiento, para planear cuánto apartar cada mes.
        </EmptyNote>
      ) : (
        <>
          <ul className="mt-3 space-y-4">
            {future.items.map((f) => {
              const pct = percentOf(f.saved, f.target);
              return (
                <li key={f.id}>
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
                    <div className="h-full rounded-full bg-saving" style={{ width: `${pct}%` }} />
                  </div>
                  <p className="mt-1 text-sm tabular-nums">
                    {formatMoney(f.saved)} de {formatMoney(f.target)} ({Math.round(pct)}%)
                  </p>
                  <p className="text-sm tabular-nums text-muted">
                    {isZero(f.remaining) ? "Meta cubierta." : `Aparta ${formatMoney(f.suggested_monthly)} al mes`}
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
      {!isZero(future.free_balance) && (
        <p className="mt-3 text-sm tabular-nums text-muted">
          Saldo libre: <span className="font-medium text-foreground">{formatMoney(future.free_balance)}</span>, ahorro en Gastos futuros sin asignar a un gasto.
        </p>
      )}
      <Link to="/configuracion" className="mt-3 inline-block text-sm font-medium underline">
        Administrar gastos futuros
      </Link>
    </section>
  );
}
