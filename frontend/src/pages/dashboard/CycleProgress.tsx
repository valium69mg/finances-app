import { Gauge } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router-dom";
import type { Dashboard } from "../../api/dashboard";
import { CardTitle } from "../../components/CardTitle";
import { cardClass } from "../../components/LinkCard";
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

/** True when spent is above the budget (a zero budget with spending counts as above). */
export function isOver(spent: string, budget: string): boolean {
  const s = toCents(spent);
  const b = toCents(budget);
  return s !== null && b !== null && s > b;
}

type Spending = Pick<Dashboard, "spending">["spending"];

/** What the card needs of the dashboard. `spending` is optional so a payload without it reads as "no fixed expenses". */
type Props = { dashboard: Pick<Dashboard, "cycle" | "categories" | "expenses"> & { spending?: Spending }; to?: string };

/** Gastado X de Y (N%) with its bar and, when `pace` is given, the within/above message. */
function PaceBlock({ spent, budget, label, day, days }: { spent: string; budget: string; label: string; day: number; days: number }) {
  const budgetCents = toCents(budget) ?? 0n;
  if (budgetCents <= 0n) return <p className="mt-3 text-sm text-muted">Sin presupuesto de gasto para comparar el ritmo.</p>;
  const pace = paceOf(spent, budgetCents, day, days);
  const pct = percentOf(spent, budget);
  return (
    <>
      <p className="mt-2 text-sm tabular-nums">
        Gastado {formatMoney(spent)} de {formatMoney(budget)} ({Math.round(pct)}%)
      </p>
      <div role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border">
        <div className={`h-full rounded-full ${pace === "above" ? "bg-destructive" : "bg-expense"}`} style={{ width: `${Math.min(100, pct)}%` }} />
      </div>
      {pace && (
        <p className={`mt-2 text-sm ${pace === "above" ? "font-medium text-destructive" : "text-muted"}`}>
          {pace === "above" ? "Vas por encima del ritmo de gasto del ciclo." : "Vas dentro del ritmo de gasto del ciclo."}
        </p>
      )}
    </>
  );
}

/** Fixed expenses are paid once, so they show what was paid against what was budgeted and never a pace. */
function FixedBlock({ spent, budget }: { spent: string; budget: string }) {
  const over = isOver(spent, budget);
  const budgetCents = toCents(budget) ?? 0n;
  const pct = budgetCents > 0n ? Math.min(100, percentOf(spent, budget)) : over ? 100 : 0;
  return (
    <div>
      <p className="text-sm font-medium">Fijos</p>
      <p className={`mt-1 text-sm tabular-nums ${over ? "font-medium text-destructive" : ""}`}>
        Pagados {formatMoney(spent)} de {formatMoney(budget)}
        {over && " (por encima del presupuesto)"}
      </p>
      <div role="progressbar" aria-label="Gastos fijos pagados" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border">
        <div className={`h-full rounded-full ${over ? "bg-destructive" : "bg-expense"}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

/**
 * Day X of Y of the displayed pay cycle. Without fixed categories: one spending block with the linear pace against the
 * month's budget (and a hint to mark the fixed ones). With them: "Fijos" (paid against budget, no pace) and "Variables"
 * (the pace, only over the variable categories).
 */
export function CycleProgress({ dashboard, to }: Props) {
  const titleId = useId();
  const { cycle, spending } = dashboard;
  const timePct = cycle.days > 0 ? Math.min(100, (cycle.day / cycle.days) * 100) : 0;
  const split = spending?.has_fixed ? spending : null;

  const budgetCents = totalBudgetCents(dashboard.categories);
  const total = `${budgetCents / 100n}.${String(budgetCents % 100n).padStart(2, "0")}`;

  // The API clamps the day to the cycle length, so a cycle that is over reads as its last day.
  const dayText = cycle.day === 0 ? "El periodo aún no empieza" : `Día ${cycle.day} de ${cycle.days}`;

  return (
    <section aria-labelledby={titleId} className={cardClass(to)}>
      <CardTitle id={titleId} icon={Gauge} to={to}>
        Progreso del ciclo
      </CardTitle>
      <p className="mt-3 text-sm tabular-nums">{dayText}</p>
      <div
        role="progressbar"
        aria-label="Avance del ciclo"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(timePct)}
        className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border"
      >
        <div className="h-full rounded-full bg-primary" style={{ width: `${timePct}%` }} />
      </div>
      {split ? (
        <div className="mt-4 space-y-4">
          <FixedBlock spent={split.fixed.spent} budget={split.fixed.budget} />
          <div>
            <p className="text-sm font-medium">Variables</p>
            <PaceBlock spent={split.variable.spent} budget={split.variable.budget} label="Gasto variable contra presupuesto" day={cycle.day} days={cycle.days} />
          </div>
        </div>
      ) : (
        <>
          <div className="mt-1">
            <PaceBlock spent={dashboard.expenses} budget={total} label="Gasto contra presupuesto total" day={cycle.day} days={cycle.days} />
          </div>
          {spending && (
            <p className="mt-3 text-sm text-muted">
              <Link to="/configuracion?seccion=categorias" className="relative z-10 font-medium underline">
                Marca tus gastos fijos en Configuración
              </Link>{" "}
              para un ritmo más realista.
            </p>
          )}
        </>
      )}
    </section>
  );
}
