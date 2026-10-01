import { useId } from "react";
import { CalendarClock } from "lucide-react";
import { Link } from "react-router-dom";
import type { UpcomingBill } from "../../api/dashboard";
import { CardTitle } from "../../components/CardTitle";
import { EmptyNote } from "../../components/EmptyNote";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

const plural = (n: number) => `${n} ${n === 1 ? "día" : "días"}`;

/** Due state as text: the red tint of an overdue bill never carries the meaning alone. */
export function dueText(b: Pick<UpcomingBill, "days_until_due" | "overdue">): string {
  if (b.overdue) return `Vencida hace ${plural(Math.abs(b.days_until_due))}`;
  if (b.days_until_due === 0) return "Vence hoy";
  if (b.days_until_due === 1) return "Vence mañana";
  return `Vence en ${plural(b.days_until_due)}`;
}

/** Bills due in the next 14 days (overdue ones included), from the bills module. */
export function UpcomingBills({ bills }: { bills: UpcomingBill[] }) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <CardTitle id={titleId} icon={CalendarClock} tone="expense">
        Próximas cuentas
      </CardTitle>
      {bills.length === 0 ? (
        <EmptyNote icon={CalendarClock}>No tienes cuentas por vencer en los próximos 14 días.</EmptyNote>
      ) : (
        <ul className="mt-2 divide-y divide-border">
          {bills.map((b) => (
            <li key={b.id} className="flex items-start justify-between gap-3 py-2">
              <div className="min-w-0">
                <p className="break-words text-sm font-medium">{b.name}</p>
                <p className={`text-xs ${b.overdue ? "font-medium text-destructive" : "text-muted"}`}>
                  {dueText(b)} · {dateLabel(b.due_date)}
                </p>
              </div>
              <span className="shrink-0 text-sm tabular-nums">
                {b.amount === null ? <span className="text-muted">Monto variable</span> : `${formatMoney(b.amount)}${b.currency === "MXN" ? "" : ` ${b.currency}`}`}
              </span>
            </li>
          ))}
        </ul>
      )}
      <Link to="/pagos-recurrentes" className="mt-3 inline-block text-sm font-medium underline">
        Ver pagos recurrentes
      </Link>
    </section>
  );
}
