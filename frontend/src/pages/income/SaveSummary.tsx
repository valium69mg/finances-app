import type { ReactNode } from "react";
import { AlertTriangle, CheckCircle2, Info } from "lucide-react";
import type { IncomeSplit, SaveIncomeResult } from "../../api/income";
import { formatMoney, formatRatePercent } from "../expenses/money";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 py-1.5">
      <dt className="min-w-0 break-words">{label}</dt>
      <dd className="min-w-0 break-words font-medium tabular-nums">{children}</dd>
    </div>
  );
}

function SplitPanel({ split }: { split: IncomeSplit }) {
  return (
    <section aria-labelledby="split-title" className="rounded-lg border border-border p-4">
      <h3 id="split-title" className="font-semibold">
        Reparto sugerido
      </h3>
      <p className="mt-1 flex items-start gap-1.5 text-muted">
        <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="min-w-0 break-words">
          Es solo una sugerencia y no se guarda. Registra tus ahorros en el módulo de Ahorros.
        </span>
      </p>
      <dl className="mt-2 divide-y divide-border">
        <Row label="Reserva SAT">{formatMoney(split.sat_reserve)}</Row>
        <Row label="Fondo de emergencia">
          {split.goal_reached ? "Meta ya alcanzada, se redirige a Inversiones" : formatMoney(split.emergency_fund)}
        </Row>
        <Row label="Inversiones">{formatMoney(split.investments)}</Row>
        {split.investment_breakdown.length > 0 && (
          <div className="py-1.5">
            <ul aria-label="Desglose de inversiones" className="space-y-0.5 pl-4 text-muted">
              {split.investment_breakdown.map((b) => (
                <li key={b.instrument} className="flex flex-wrap justify-between gap-x-4">
                  <span className="min-w-0 break-words">{b.instrument.toUpperCase()}</span>
                  <span className="tabular-nums">{formatMoney(b.amount)}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
        <Row label="Aguinaldo y vacaciones">{formatMoney(split.aguinaldo_vacation)}</Row>
      </dl>
    </section>
  );
}

/** Summary shown after a save: month total, estimated RESICO ISR, rate warning and the suggested split. */
export function SaveSummary({ result }: { result: SaveIncomeResult }) {
  const { summary, split } = result;
  const resico = summary.resico;
  return (
    <div role="status" className="space-y-3 text-sm">
      <span className="flex items-center gap-1.5">
        <CheckCircle2 className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
        Ingreso guardado.
      </span>
      <dl className="divide-y divide-border rounded-lg border border-border px-4">
        <Row label={`Total del mes (${summary.month})`}>{formatMoney(summary.month_total_mxn)} MXN</Row>
        {resico ? (
          <Row label="ISR RESICO estimado">
            {formatMoney(resico.estimated_isr)} ({formatRatePercent(resico.rate)})
          </Row>
        ) : (
          <Row label="ISR RESICO estimado">Sin estimación</Row>
        )}
      </dl>
      {resico?.rate_increased && (
        <p className="flex items-start gap-1.5 font-medium text-destructive">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 break-words">
            Este ingreso te movió a una tasa RESICO mayor
            {resico.previous_rate ? `: ${formatRatePercent(resico.previous_rate)} → ${formatRatePercent(resico.rate)}` : ""}.
          </span>
        </p>
      )}
      {split && <SplitPanel split={split} />}
    </div>
  );
}
