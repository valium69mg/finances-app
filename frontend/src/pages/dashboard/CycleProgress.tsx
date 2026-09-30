import { useId } from "react";
import type { Dashboard } from "../../api/dashboard";
import { formatMoney, percentOf } from "../expenses/money";

/** Cents of a plain decimal string, or null when it is not one. Integer math only. */
function toCents(v: string): bigint | null {
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(v.trim());
  if (!m) return null;
  const cents = BigInt(m[2] + (m[3] ?? "").padEnd(2, "0").slice(0, 2));
  return m[1] ? -cents : cents;
}

/** Sum of the category budgets that exist, in cents. */
export function totalBudgetCents(categories: Dashboard["categories"]): bigint {
  return categories.reduce((sum, c) => sum + (c.budget === null ? 0n : (toCents(c.budget) ?? 0n)), 0n);
}

export type Pace = "within" | "above";

/**
 * Spent against the budget of the days elapsed: above the pace when spent/budget is ahead of day/days, compared
 * by cross-multiplying so no division or float is involved.
 */
export function paceOf(spent: string, budgetCents: bigint, day: number, days: number): Pace | null {
  const s = toCents(spent);
  if (s === null || budgetCents <= 0n || days <= 0) return null;
  return s * BigInt(days) > budgetCents * BigInt(day) ? "above" : "within";
}

/** Day X of Y of the displayed pay cycle, with the spending pace against the month's budget. */
export function CycleProgress({ dashboard }: { dashboard: Pick<Dashboard, "cycle" | "categories" | "expenses"> }) {
  const titleId = useId();
  const { cycle } = dashboard;
  const budgetCents = totalBudgetCents(dashboard.categories);
  const budget = `${budgetCents / 100n}.${String(budgetCents % 100n).padStart(2, "0")}`;
  const pace = paceOf(dashboard.expenses, budgetCents, cycle.day, cycle.days);
  const timePct = cycle.days > 0 ? Math.min(100, (cycle.day / cycle.days) * 100) : 0;
  const spentPct = percentOf(dashboard.expenses, budget);

  // The API clamps the day to the cycle length, so a cycle that is over reads as its last day.
  const dayText = cycle.day === 0 ? "El periodo aún no empieza" : `Día ${cycle.day} de ${cycle.days}`;

  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <h3 id={titleId} className="font-semibold">
        Progreso del ciclo
      </h3>
      <p className="mt-1 text-sm tabular-nums">{dayText}</p>
      <div
        role="progressbar"
        aria-label="Avance del ciclo"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(timePct)}
        className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border"
      >
        <div className="h-full rounded-full bg-accent" style={{ width: `${timePct}%` }} />
      </div>
      {budgetCents > 0n ? (
        <>
          <p className="mt-3 text-sm tabular-nums">
            Gastado {formatMoney(dashboard.expenses)} de {formatMoney(budget)} ({Math.round(spentPct)}%)
          </p>
          <div
            role="progressbar"
            aria-label="Gasto contra presupuesto total"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(spentPct)}
            className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border"
          >
            <div className={`h-full rounded-full ${pace === "above" ? "bg-destructive" : "bg-accent"}`} style={{ width: `${spentPct}%` }} />
          </div>
          {pace && (
            <p className={`mt-2 text-sm ${pace === "above" ? "font-medium text-destructive" : "text-muted"}`}>
              {pace === "above" ? "Vas por encima del ritmo de gasto del ciclo." : "Vas dentro del ritmo de gasto del ciclo."}
            </p>
          )}
        </>
      ) : (
        <p className="mt-3 text-sm text-muted">Sin presupuesto de gasto para comparar el ritmo.</p>
      )}
    </section>
  );
}
