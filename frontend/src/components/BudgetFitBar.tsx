import { AlertTriangle, CheckCircle2, Info } from "lucide-react";
import type { BudgetCheck } from "../api/expenseRequests";
import { fitSegments } from "../lib/budgetFit";
import { formatMoney } from "../pages/expenses/money";

/** Diagonal stripes: the request is a projection, not money spent yet. Pattern, label and legend say so, never color alone. */
const stripes = (channel: "--color-expense" | "--color-destructive"): { backgroundImage: string } => ({
  backgroundImage: `repeating-linear-gradient(135deg, rgb(var(${channel}) / 0.55) 0 5px, rgb(var(${channel}) / 0.18) 5px 10px)`,
});

const Swatch = ({ className, style }: { className?: string; style?: { backgroundImage: string } }) => (
  <span aria-hidden="true" className={`inline-block h-3 w-5 shrink-0 rounded-sm border border-foreground/10 ${className ?? ""}`} style={style} />
);

/**
 * The budget usage of a category as it WOULD look after approving a request: what is already spent (solid), the
 * request on top of it (striped, lighter, and labelled), the budget limit, and the excess in the destructive color
 * with its amount in words. A request that does not fit is still approvable, so this only informs.
 */
export function BudgetFitBar({ check }: { check: BudgetCheck }) {
  const { budget, spent, remaining, amount, projected_spent: projected, fits, over_by: overBy } = check;
  const segments = budget === null ? null : fitSegments(budget, spent, amount);

  if (budget === null || segments === null) {
    return (
      <div data-testid="budget-fit" role="status" aria-live="polite" className="space-y-1.5 rounded-lg border border-border bg-background p-3 text-sm">
        <p className="flex items-center gap-2 font-medium">
          <Info className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
          Sin presupuesto para esta categoría
        </p>
        <p className="tabular-nums text-muted">
          Gastado {formatMoney(spent)} · Con esta petición: {formatMoney(projected)}
        </p>
      </div>
    );
  }

  const exceeds = fits === false;
  const label = exceeds
    ? `Con esta petición el gasto de ${check.category} sería ${formatMoney(projected)} de ${formatMoney(budget)}: excede el presupuesto por ${formatMoney(overBy)}.`
    : `Con esta petición el gasto de ${check.category} sería ${formatMoney(projected)} de ${formatMoney(budget)}: cabe en el presupuesto.`;
  const left = remaining ?? "0";
  const leftNegative = left.startsWith("-");

  return (
    <div data-testid="budget-fit" role="status" aria-live="polite" className={`space-y-2.5 rounded-lg border p-3 text-sm ${exceeds ? "border-destructive/40 bg-destructive/5" : "border-border bg-background"}`}>
      <div role="img" aria-label={label} className="relative h-3.5 w-full overflow-hidden rounded-full bg-border">
        <div className="flex h-full w-full">
          <div className="h-full bg-expense" style={{ width: `${segments.spentWithin}%` }} data-segment="spent" />
          <div className="h-full" style={{ ...stripes("--color-expense"), width: `${segments.requestWithin}%` }} data-segment="request" />
          <div className="h-full bg-destructive" style={{ width: `${segments.spentOver}%` }} data-segment="spent-over" />
          <div className="h-full" style={{ ...stripes("--color-destructive"), width: `${segments.requestOver}%` }} data-segment="request-over" />
        </div>
        {segments.budgetMarker !== null && (
          <span aria-hidden="true" data-segment="limit" className="absolute inset-y-0 w-0.5 bg-foreground" style={{ left: `calc(${segments.budgetMarker}% - 1px)` }} />
        )}
      </div>

      <ul aria-label="Leyenda de la barra" className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
        <li className="flex items-center gap-1.5">
          <Swatch className="bg-expense" />
          Gastado
        </li>
        <li className="flex items-center gap-1.5">
          <Swatch style={stripes("--color-expense")} />
          Esta petición
        </li>
        {exceeds && (
          <li className="flex items-center gap-1.5">
            <Swatch style={stripes("--color-destructive")} />
            Excedente
          </li>
        )}
        {segments.budgetMarker !== null && (
          <li className="flex items-center gap-1.5">
            <span aria-hidden="true" className="inline-block h-3 w-0.5 bg-foreground" />
            Límite del presupuesto
          </li>
        )}
      </ul>

      <p className="tabular-nums">
        Presupuesto {formatMoney(budget)} · Gastado {formatMoney(spent)} ·{" "}
        <span className={leftNegative ? "font-medium text-destructive" : undefined}>Restante {formatMoney(left)}</span>
      </p>
      <p className="tabular-nums">
        Con esta petición: {formatMoney(projected)} de {formatMoney(budget)}
      </p>
      {exceeds ? (
        <p className="flex items-start gap-2 font-medium text-destructive">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 break-words">Excede el presupuesto por {formatMoney(overBy)}. Aun así puedes aprobarla.</span>
        </p>
      ) : (
        <p className="flex items-start gap-2 font-medium">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-income" aria-hidden="true" />
          <span className="min-w-0 break-words">Cabe en el presupuesto</span>
        </p>
      )}
    </div>
  );
}
