import type { LucideIcon } from "lucide-react";
import { CircleDollarSign } from "lucide-react";
import type { ReactNode } from "react";
import type { Dashboard } from "../../api/dashboard";
import { IconChip } from "../../components/IconChip";
import { KIND, TONES, type Tone } from "../../lib/tones";
import { formatMoney } from "../expenses/money";

function Card({ label, hint, negative, icon, tone, children }: { label: string; hint?: string; negative?: boolean; icon: LucideIcon; tone: Tone; children: ReactNode }) {
  return (
    <div className={`flex min-w-0 items-start gap-3 rounded-lg border p-4 ${TONES[tone].card}`}>
      <IconChip icon={icon} tone={tone} onSoft />
      <div className="min-w-0 flex-1">
        <dt className="text-sm text-muted">{label}</dt>
        <dd className={`mt-1 break-words text-xl font-semibold tabular-nums ${negative ? "text-destructive" : ""}`}>{children}</dd>
        {hint && <p className="mt-1 text-xs text-muted">{hint}</p>}
      </div>
    </div>
  );
}

/** Month totals: income, expenses, savings contributed and the money left. Each kind has its own icon and tint. */
export function TotalsCards({ dashboard }: { dashboard: Pick<Dashboard, "income" | "expenses" | "savings" | "available"> }) {
  return (
    <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card label="Ingresos" icon={KIND.Ingreso.icon} tone={KIND.Ingreso.tone}>
        {formatMoney(dashboard.income)}
      </Card>
      <Card label="Gastos" icon={KIND.Gasto.icon} tone={KIND.Gasto.tone}>
        {formatMoney(dashboard.expenses)}
      </Card>
      <Card label="Ahorros" hint="Aportaciones netas del mes" icon={KIND.Ahorro.icon} tone={KIND.Ahorro.tone}>
        {formatMoney(dashboard.savings)}
      </Card>
      <Card label="Disponible" hint="Ingresos − gastos − ahorros" icon={CircleDollarSign} tone="primary" negative={dashboard.available.startsWith("-")}>
        {formatMoney(dashboard.available)}
      </Card>
    </dl>
  );
}
