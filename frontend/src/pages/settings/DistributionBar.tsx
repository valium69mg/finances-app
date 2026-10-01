import { AlertTriangle, CircleDashed, PieChart } from "lucide-react";
import { CardTitle } from "../../components/CardTitle";
import { IconChip } from "../../components/IconChip";
import { KIND, TONES } from "../../lib/tones";
import { formatPlain, type Distribution } from "../../lib/planning";
import { formatMoney } from "../expenses/money";

interface Props {
  distribution: Distribution;
  salaryUsd: string;
  fxRate: string;
}

const pct = (v: string) => `${v.replace(/^-/, "")}\u00A0%`;

/** Budget distribution against the base monthly income: assigned (Gasto violet, Ahorro sky) and what is left. */
export function DistributionBar({ distribution: d, salaryUsd, fxRate }: Props) {
  const remaining = d.remaining.startsWith("-") ? d.remaining.slice(1) : d.remaining;
  const Expense = KIND.Gasto;
  const Saving = KIND.Ahorro;
  return (
    <section aria-labelledby="distribution-title" className="rounded-xl border border-border bg-background p-4 sm:p-5">
      <CardTitle id="distribution-title" icon={PieChart}>
        Distribución del ingreso base
      </CardTitle>
      <p className="mt-2 text-sm">
        <span className={`font-semibold ${d.over ? "text-destructive" : ""}`}>{formatMoney(d.assigned)}</span> asignados de {formatMoney(d.base)} ·{" "}
        <span className={`font-semibold ${d.over ? "text-destructive" : ""}`}>{pct(d.assignedPct)}</span>
      </p>

      <div
        role="progressbar"
        aria-label="Presupuestos asignados del ingreso base"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.min(100, Number(d.assignedPct))}
        aria-valuetext={`${pct(d.assignedPct)} asignado`}
        className={`mt-3 flex h-3 overflow-hidden rounded-full border bg-border/40 ${d.over ? "border-destructive" : "border-border"}`}
      >
        <div className={d.over ? "bg-destructive" : TONES.expense.bar} style={{ width: `${d.expenseWidth}%` }} />
        <div className={d.over ? "bg-destructive/70" : TONES.saving.bar} style={{ width: `${d.savingWidth}%` }} />
      </div>

      <dl className="mt-3 grid gap-2 text-sm sm:grid-cols-3">
        <div className="flex min-w-0 items-center gap-2">
          <IconChip icon={Expense.icon} tone={Expense.tone} size="sm" />
          <div className="min-w-0">
            <dt className="text-muted">Gasto</dt>
            <dd className="font-medium">{formatMoney(d.expense)}</dd>
          </div>
        </div>
        <div className="flex min-w-0 items-center gap-2">
          <IconChip icon={Saving.icon} tone={Saving.tone} size="sm" />
          <div className="min-w-0">
            <dt className="text-muted">Ahorro</dt>
            <dd className="font-medium">{formatMoney(d.saving)}</dd>
          </div>
        </div>
        <div className="flex min-w-0 items-center gap-2">
          <span aria-hidden="true" className={`inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${d.over ? "bg-destructive/10 text-destructive" : "bg-primary/10 text-primary"}`}>
            {d.over ? <AlertTriangle className="h-4 w-4" /> : <CircleDashed className="h-4 w-4" />}
          </span>
          <div className="min-w-0">
            <dt className={d.over ? "font-medium text-destructive" : "text-muted"}>{d.over ? "Excedido" : "Sin asignar"}</dt>
            <dd className={`font-medium ${d.over ? "text-destructive" : ""}`}>
              {formatMoney(remaining)} · {pct(d.remainingPct)}
            </dd>
          </div>
        </div>
      </dl>

      <p aria-live="polite" className={`mt-3 text-sm ${d.over ? "font-medium text-destructive" : "sr-only"}`}>
        {d.over ? `Los presupuestos exceden el ingreso base por ${formatMoney(d.excess)}.` : ""}
      </p>
      <p className="mt-2 text-sm text-muted">
        Base: {formatPlain(salaryUsd)} USD × {formatPlain(fxRate)} = {formatMoney(d.base)}. Es una estimación para planear, no el depósito real.
      </p>
      <p className="mt-1 text-sm text-muted">Usa los presupuestos normales: no incluye los ajustes por mes ni el plan de pausa de inversiones.</p>
    </section>
  );
}
