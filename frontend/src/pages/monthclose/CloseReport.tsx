import { AlertTriangle } from "lucide-react";
import type { CloseSuggestion, MonthClose } from "../../api/monthClose";
import { formatMoney } from "../expenses/money";
import { BudgetTable } from "../dashboard/BudgetTable";
import { TotalsCards } from "../dashboard/TotalsCards";
import { EmergencyCard } from "../savings/PortfolioPanel";
import { FilingStatusBadge } from "../taxfiling/StatusBadge";
import { periodLabel } from "../taxfiling/labels";
import { describeAdjustment, overBudgetOf } from "./labels";

function OverBudget({ close }: { close: MonthClose }) {
  const over = overBudgetOf(close);
  if (over.length === 0) return <p className="text-sm">Ninguna categoría de gasto excedió su presupuesto.</p>;
  return (
    <ul aria-label="Categorías sobre presupuesto" className="divide-y divide-border rounded-xl border border-border">
      {over.map((c) => (
        <li key={c.category} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-3 text-sm">
          <span className="flex min-w-0 items-center gap-2 font-medium">
            <AlertTriangle className="h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
            <span className="break-words">{c.category}</span>
          </span>
          <span className="tabular-nums">
            {formatMoney(c.spent)} contra presupuesto de {c.budget === null ? "—" : formatMoney(c.budget)}
          </span>
        </li>
      ))}
    </ul>
  );
}

const isZero = (v: string) => !/[1-9]/.test(v);

function Suggestion({ suggestion, goal }: { suggestion: CloseSuggestion | null; goal: string }) {
  if (suggestion === null) {
    return <p className="text-sm">No quedó dinero disponible este mes, así que no hay nada que repartir.</p>;
  }
  const lines: { label: string; amount: string; note?: string }[] = [];
  if (!isZero(suggestion.to_emergency_fund)) {
    lines.push({ label: "Fondo de emergencia", amount: suggestion.to_emergency_fund, note: `Meta: ${formatMoney(goal)}` });
  }
  if (!isZero(suggestion.to_investments)) lines.push({ label: "Inversiones", amount: suggestion.to_investments });
  if (!isZero(suggestion.to_future_expenses)) {
    lines.push({ label: "Gastos futuros", amount: suggestion.to_future_expenses, note: "Inversiones está en pausa este mes" });
  }
  return (
    <div className="space-y-2">
      <ul aria-label="Sugerencia del dinero disponible" className="divide-y divide-border rounded-xl border border-border">
        {lines.map((l) => (
          <li key={l.label} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-3 text-sm">
            <span className="min-w-0">
              <span className="font-medium">Mover a {l.label}</span>
              {l.note && <span className="block text-xs text-muted">{l.note}</span>}
            </span>
            <span className="font-semibold tabular-nums">{formatMoney(l.amount)}</span>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted">Es solo una sugerencia: no se registra ningún ahorro por ti. Regístralos en Ahorros si sigues el consejo.</p>
    </div>
  );
}

function Adjustments({ close }: { close: MonthClose }) {
  if (close.adjustments.length === 0) return <p className="text-sm">Ningún presupuesto se desvió más de 20% de lo real.</p>;
  return (
    <div className="space-y-2">
      <ul aria-label="Ajustes de presupuesto sugeridos" className="divide-y divide-border rounded-xl border border-border">
        {close.adjustments.map((a) => (
          <li key={`${a.kind}-${a.category}`} className="px-4 py-3 text-sm">
            <p className="break-words font-medium">
              {a.category} <span className="font-normal text-muted">({a.kind === "Gasto" ? "gasto" : "ahorro"})</span>
            </p>
            <p className="mt-0.5 tabular-nums">{describeAdjustment(a)}</p>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted">Considera ajustar estos presupuestos en Configuración. La app no los cambia por ti.</p>
    </div>
  );
}

const h3 = "mb-3 text-base font-semibold tracking-tight";

/** The figures of a close, shared by the preview and the stored snapshot: totals, over-budget list, budget table, emergency fund, suggestion, hints and filing status. */
export function CloseReport({ close }: { close: MonthClose }) {
  return (
    <div className="space-y-6">
      <section aria-label="Resumen del mes">
        <TotalsCards dashboard={close} />
      </section>

      <section aria-label="Categorías sobre presupuesto">
        <h3 className={h3}>Categorías sobre presupuesto</h3>
        <OverBudget close={close} />
      </section>

      <section aria-label="Presupuesto por categoría">
        <h3 className={h3}>Presupuesto por categoría</h3>
        {close.categories.length === 0 ? (
          <p className="text-sm text-muted">No hay categorías de gasto en Configuración.</p>
        ) : (
          <BudgetTable categories={close.categories} />
        )}
      </section>

      <div className="grid gap-4 md:grid-cols-2">
        <EmergencyCard emergency={close.emergency} />
        {close.tax_filing_status !== null && (
          <section aria-label="Declaración del mes" className="rounded-lg border border-border p-4">
            <h3 className="font-semibold">Declaración de {periodLabel(close.period)}</h3>
            <p className="mt-2">
              <FilingStatusBadge status={close.tax_filing_status} />
            </p>
            <p className="mt-2 text-xs text-muted">Estado de la declaración de este periodo {close.closed_at === null ? "hoy" : "al generar el cierre"}.</p>
          </section>
        )}
      </div>

      <section aria-label="Dinero disponible">
        <h3 className={h3}>¿Dónde poner el dinero disponible?</h3>
        <Suggestion suggestion={close.suggestion} goal={close.emergency.goal} />
      </section>

      <section aria-label="Ajustes de presupuesto">
        <h3 className={h3}>Ajustes de presupuesto sugeridos</h3>
        <Adjustments close={close} />
      </section>
    </div>
  );
}
