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
 * Two columns on a phone (a 2x2 grid), four from lg up; `compact` keeps two columns where the page is narrow (Hogar, beside its section nav).
 * `links` makes a card a link (the household and the owner's Panels pass them).
 */
export function TotalsCards({ dashboard, links = {}, compact = false }: { dashboard: Pick<Dashboard, "income" | "expenses" | "savings" | "available">; links?: TotalsLinks; compact?: boolean }) {
  return (
    <dl className={`grid grid-cols-2 gap-3 sm:gap-4 ${compact ? "" : "lg:grid-cols-4"}`}>
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
