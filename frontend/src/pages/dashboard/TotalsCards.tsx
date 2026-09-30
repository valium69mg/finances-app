import type { ReactNode } from "react";
import type { Dashboard } from "../../api/dashboard";
import { formatMoney } from "../expenses/money";

function Card({ label, hint, negative, children }: { label: string; hint?: string; negative?: boolean; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-lg border border-border p-4">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className={`mt-1 break-words text-xl font-semibold tabular-nums ${negative ? "text-destructive" : ""}`}>{children}</dd>
      {hint && <p className="mt-1 text-xs text-muted">{hint}</p>}
    </div>
  );
}

/** Month totals: income, expenses, savings contributed and the money left. */
export function TotalsCards({ dashboard }: { dashboard: Dashboard }) {
  return (
    <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card label="Ingresos">{formatMoney(dashboard.income)}</Card>
      <Card label="Gastos">{formatMoney(dashboard.expenses)}</Card>
      <Card label="Ahorros" hint="Aportaciones netas del mes">
        {formatMoney(dashboard.savings)}
      </Card>
      <Card label="Disponible" hint="Ingresos − gastos − ahorros" negative={dashboard.available.startsWith("-")}>
        {formatMoney(dashboard.available)}
      </Card>
    </dl>
  );
}
