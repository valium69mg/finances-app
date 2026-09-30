import { AlertTriangle, CheckCircle2, Info } from "lucide-react";
import type { SaveExpenseResult } from "../../api/expenses";
import { formatMoney } from "./money";

/** Budget feedback shown after a save: "X de Y", an over-budget warning or "no budget". */
export function BudgetFeedbackNote({ result }: { result: SaveExpenseResult }) {
  const { budget, expense } = result;
  const saved = (
    <span className="flex items-center gap-1.5">
      <CheckCircle2 className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
      Gasto guardado.
    </span>
  );

  if (!budget || budget.budget === null) {
    const category = budget?.category || expense.category;
    return (
      <div role="status" className="space-y-1 text-sm">
        {saved}
        <p className="flex items-start gap-1.5 text-muted">
          <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 break-words">Sin presupuesto definido{category ? ` para ${category}` : ""}.</span>
        </p>
      </div>
    );
  }

  const over = budget.over_budget;
  const excess = budget.remaining?.replace(/^-/, "") ?? "0";
  return (
    <div role="status" className="space-y-1 text-sm">
      {saved}
      <p className="min-w-0 break-words">
        {budget.category}: {formatMoney(budget.spent)} de {formatMoney(budget.budget)} en {budget.month}
      </p>
      {over && (
        <p className="flex items-start gap-1.5 font-medium text-destructive">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 break-words">Te pasaste del presupuesto por {formatMoney(excess)}.</span>
        </p>
      )}
    </div>
  );
}
