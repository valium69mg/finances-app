import { AlertTriangle } from "lucide-react";
import type { DashboardCategory } from "../../api/dashboard";
import { formatMoney, percentOf } from "../expenses/money";

const th = "px-3 py-2 text-left text-sm font-medium text-muted";
const thNum = "px-3 py-2 text-right text-sm font-medium text-muted";
const td = "px-3 py-2 align-top";
const tdNum = "px-3 py-2 text-right align-top tabular-nums";

/** Width of the usage bar: full when over budget (a zero budget has no meaningful share). */
function usage(row: DashboardCategory): number {
  if (row.budget === null) return 0;
  return row.over_budget ? 100 : percentOf(row.spent, row.budget);
}

function UsageBar({ row }: { row: DashboardCategory }) {
  if (row.budget === null) return <span className="text-muted">Sin presupuesto</span>;
  const pct = usage(row);
  return (
    <div className="flex items-center gap-2">
      <div
        role="progressbar"
        aria-label={`Uso del presupuesto de ${row.category}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(pct)}
        className="h-2.5 min-w-16 flex-1 overflow-hidden rounded-full bg-border"
      >
        <div className={`h-full rounded-full ${row.over_budget ? "bg-destructive" : "bg-accent"}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="w-14 shrink-0 text-right tabular-nums">{Math.round(pct)}%</span>
    </div>
  );
}

/** Usage bar across the full card width, or the "no budget" note. */
function CardBar({ row }: { row: DashboardCategory }) {
  if (row.budget === null) return <p className="mt-2 text-sm text-muted">Sin presupuesto</p>;
  const pct = usage(row);
  return (
    <div
      role="progressbar"
      aria-label={`Uso del presupuesto de ${row.category}`}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(pct)}
      className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border"
    >
      <div className={`h-full rounded-full ${row.over_budget ? "bg-destructive" : "bg-accent"}`} style={{ width: `${pct}%` }} />
    </div>
  );
}

/** Phone layout: one compact card per category (name and percent, bar, spent of budget and what is left). */
function BudgetCards({ categories }: { categories: DashboardCategory[] }) {
  return (
    <ul aria-label="Presupuesto contra gasto real por categoría" className="space-y-3 md:hidden">
      {categories.map((c) => (
        <li key={c.category} data-testid="budget-card" className={`rounded-xl border p-3 ${c.over_budget ? "border-destructive/40 bg-destructive/5" : "border-border"}`}>
          <div className="flex items-baseline justify-between gap-3">
            <span className="min-w-0 break-words font-medium">{c.category}</span>
            {c.budget !== null && <span className="shrink-0 text-sm tabular-nums">{Math.round(usage(c))}%</span>}
          </div>
          <CardBar row={c} />
          <div className="mt-2 flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
            <span className="min-w-0 tabular-nums">
              Gastado {formatMoney(c.spent)}
              {c.budget !== null && <> de {formatMoney(c.budget)}</>}
            </span>
            {c.remaining !== null && (
              <span className={`tabular-nums ${c.over_budget ? "font-medium text-destructive" : "text-muted"}`}>
                {c.over_budget ? "Excedido" : "Restante"} {formatMoney(c.over_budget ? c.remaining.replace(/^-/, "") : c.remaining)}
              </span>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
}

/**
 * Budget versus actual per Gasto category for the selected month; exceeded budgets are flagged in text and
 * color. Compact cards below the md breakpoint (no horizontal scroll on a phone), a table from md up.
 */
export function BudgetTable({ categories }: { categories: DashboardCategory[] }) {
  return (
    <>
      <BudgetCards categories={categories} />
      <BudgetGrid categories={categories} />
    </>
  );
}

function BudgetGrid({ categories }: { categories: DashboardCategory[] }) {
  return (
    <div className="hidden overflow-x-auto rounded-xl border border-border md:block">
      <table className="w-full min-w-[40rem] border-collapse text-sm">
        <caption className="sr-only">Presupuesto contra gasto real por categoría</caption>
        <thead>
          <tr className="border-b border-border">
            <th scope="col" className={th}>
              Categoría
            </th>
            <th scope="col" className={thNum}>
              Gastado
            </th>
            <th scope="col" className={thNum}>
              Presupuesto
            </th>
            <th scope="col" className={thNum}>
              Restante
            </th>
            <th scope="col" className={`${th} w-56`}>
              Uso
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {categories.map((c) => (
            <tr key={c.category} className={c.over_budget ? "bg-destructive/5" : undefined}>
              <th scope="row" className={`${td} text-left font-medium`}>
                <span className="break-words">{c.category}</span>
                {c.over_budget && (
                  <span className="mt-0.5 flex items-center gap-1 text-xs font-medium text-destructive">
                    <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                    Presupuesto excedido
                  </span>
                )}
              </th>
              <td className={tdNum}>{formatMoney(c.spent)}</td>
              <td className={tdNum}>{c.budget === null ? <span className="text-muted">—</span> : formatMoney(c.budget)}</td>
              <td className={`${tdNum} ${c.over_budget ? "font-medium text-destructive" : ""}`}>
                {c.remaining === null ? <span className="text-muted">—</span> : formatMoney(c.remaining)}
              </td>
              <td className={td}>
                <UsageBar row={c} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
