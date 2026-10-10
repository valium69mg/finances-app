import { CircleDollarSign } from "lucide-react";
import type { Dashboard } from "../../api/dashboard";
import { StatCard } from "../../components/LinkCard";
import { KIND } from "../../lib/tones";
import { formatMoney } from "../expenses/money";

/** Where each clickable card goes. A card without a target is informative; Disponible never has one. */
export interface TotalsLinks {
  income?: string;
  expenses?: string;
  savings?: string;
}

/**
 * Month totals: income, expenses, savings contributed and the money left. Each kind has its own icon and tint.
 * `links` makes a card a link (the owner's Panel passes none for now).
 */
export function TotalsCards({ dashboard, links = {} }: { dashboard: Pick<Dashboard, "income" | "expenses" | "savings" | "available">; links?: TotalsLinks }) {
  return (
    <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <StatCard label="Ingresos" icon={KIND.Ingreso.icon} tone={KIND.Ingreso.tone} to={links.income}>
        {formatMoney(dashboard.income)}
      </StatCard>
      <StatCard label="Gastos" icon={KIND.Gasto.icon} tone={KIND.Gasto.tone} to={links.expenses}>
        {formatMoney(dashboard.expenses)}
      </StatCard>
      <StatCard label="Ahorros" hint="Aportaciones netas del mes" icon={KIND.Ahorro.icon} tone={KIND.Ahorro.tone} to={links.savings}>
        {formatMoney(dashboard.savings)}
      </StatCard>
      <StatCard label="Disponible" hint="Ingresos − gastos − ahorros" icon={CircleDollarSign} tone="primary" negative={dashboard.available.startsWith("-")}>
        {formatMoney(dashboard.available)}
      </StatCard>
    </dl>
  );
}
