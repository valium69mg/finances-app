import { AlertTriangle } from "lucide-react";
import { Link } from "react-router-dom";
import type { DashboardCategory } from "../../api/dashboard";
import { IconChip } from "../../components/IconChip";
import { Chevron, ROW_LINK, ROW_LINKED } from "../../components/LinkCard";
import { categoryIcon } from "../../lib/categoryIcon";
import { formatMoney, percentOf } from "../expenses/money";

const th = "px-3 py-2 text-left text-sm font-medium text-muted";
const thNum = "px-3 py-2 text-right text-sm font-medium text-muted";
const td = "px-3 py-2 align-top";
const tdNum = "px-3 py-2 text-right align-top tabular-nums";

/** Fill of a usage bar: the Gasto color, red only once the budget is exceeded (the text flag says so as well). */
const barColor = (row: DashboardCategory) => (row.over_budget ? "bg-destructive" : "bg-expense");

/** Target of a clickable row, or undefined for a row that is not a link. */
export type RowLink = (row: DashboardCategory) => string | undefined;

/** The category name; a link stretched over the whole row when the row has a target. */
function RowName({ to, children }: { to: string | undefined; children: string }) {
  if (!to) return <span className="min-w-0 break-words">{children}</span>;
  return (
    <Link to={to} className={`${ROW_LINK} min-w-0 break-words after:rounded-lg`}>
      {children}
    </Link>
  );
}

function CategoryChip({ name }: { name: string }) {
  return <IconChip icon={categoryIcon(name)} tone="expense" size="sm" />;
}

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
        <div className={`h-full rounded-full ${barColor(row)}`} style={{ width: `${pct}%` }} />
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
      <div className={`h-full rounded-full ${barColor(row)}`} style={{ width: `${pct}%` }} />
    </div>
  );
}

/** Phone layout: one compact card per category (name and percent, bar, spent of budget and what is left). */
function BudgetCards({ categories, rowLink }: { categories: DashboardCategory[]; rowLink?: RowLink }) {
  return (
    <ul aria-label="Presupuesto contra gasto real por categoría" className="space-y-3 md:hidden">
      {categories.map((c) => {
        const to = rowLink?.(c);
        return (
          <li
            key={c.category}
            data-testid="budget-card"
            className={`rounded-xl border p-3 ${c.over_budget ? "border-destructive/40 bg-destructive/5" : "border-border"} ${to ? `${ROW_LINKED} hover:border-foreground/30` : ""}`}
          >
            <div className="flex items-center justify-between gap-3">
              <span className="flex min-w-0 items-center gap-2">
                <CategoryChip name={c.category} />
                <span className="min-w-0 font-medium">
                  <RowName to={to}>{c.category}</RowName>
                </span>
              </span>
              <span className="flex shrink-0 items-center gap-1">
                {c.budget !== null && <span className="text-sm tabular-nums">{Math.round(usage(c))}%</span>}
                {to && <Chevron />}
              </span>
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
        );
      })}
    </ul>
  );
}

/**
 * Budget versus actual per Gasto category for the selected month; exceeded budgets are flagged in text and
 * color. Compact cards below the md breakpoint (no horizontal scroll on a phone), a table from md up.
 */
export function BudgetTable({ categories, rowLink }: { categories: DashboardCategory[]; rowLink?: RowLink }) {
  return (
    <>
      <BudgetCards categories={categories} rowLink={rowLink} />
      <BudgetGrid categories={categories} rowLink={rowLink} />
    </>
  );
}

function BudgetGrid({ categories, rowLink }: { categories: DashboardCategory[]; rowLink?: RowLink }) {
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
          {categories.map((c) => {
            const to = rowLink?.(c);
            return (
              <tr key={c.category} className={[c.over_budget && "bg-destructive/5", to && ROW_LINKED].filter(Boolean).join(" ") || undefined}>
                <th scope="row" className={`${td} text-left font-medium`}>
                  <span className="flex items-center gap-2">
                    <CategoryChip name={c.category} />
                    <RowName to={to}>{c.category}</RowName>
                  </span>
                  {c.over_budget && (
                    <span className="mt-1 flex items-center gap-1 text-xs font-medium text-destructive">
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
                  {to ? (
                    <div className="flex items-center gap-2">
                      <div className="min-w-0 flex-1">
                        <UsageBar row={c} />
                      </div>
                      <Chevron />
                    </div>
                  ) : (
                    <UsageBar row={c} />
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
