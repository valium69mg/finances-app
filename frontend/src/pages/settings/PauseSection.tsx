import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { PauseCircle, Pencil, PlayCircle } from "lucide-react";
import { getMonthBudgets, settingsKeys, type Pause } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { formatMoney, todayISO } from "../expenses/money";
import { PauseDialog } from "./PauseDialog";
import { formatMonthName } from "./summary";
import { Card, ErrorBanner, SectionHeader, describeSaveError, secondaryButton } from "./ui";
import { isMonth } from "./validation";

function MonthPreview() {
  const [month, setMonth] = useState(() => todayISO().slice(0, 7));
  const valid = isMonth(month);
  const budgets = useQuery({ queryKey: settingsKeys.budgets(month), queryFn: () => getMonthBudgets(month), enabled: valid, retry: false });

  return (
    <Card>
      <h3 className="font-semibold">Presupuesto efectivo del mes</h3>
      <p className="mt-1 text-sm text-muted">Así queda tu presupuesto en un mes, con la pausa aplicada.</p>
      <div className="mt-3 max-w-xs">
        <TextField label="Mes a consultar" type="month" value={month} onChange={(e) => setMonth(e.target.value)} error={valid ? null : "Elige un mes válido."} />
      </div>
      <div className="mt-4" aria-live="polite">
        {budgets.isPending && valid && <p className="text-sm text-muted">Cargando…</p>}
        {budgets.isError && <ErrorBanner>{describeSaveError(budgets.error)}</ErrorBanner>}
        {budgets.data && (
          <ul aria-label={`Presupuestos de ${month}`} className="divide-y divide-border text-sm">
            {budgets.data.map((b) => (
              <li key={b.name} className="flex items-baseline justify-between gap-3 py-2">
                <span className="min-w-0 break-words">{b.name}</span>
                <span className="shrink-0 tabular-nums">{b.budget === null ? "Sin presupuesto" : `$${b.budget}`}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Card>
  );
}

/** Status of the investment pause first; the effective budget of a month is a secondary lookup. */
export function PauseSection({ data }: { data: Pause | null }) {
  const [editing, setEditing] = useState(false);
  return (
    <div className="space-y-6">
      <SectionHeader title="Pausa de inversiones" description="Pausa tus inversiones durante algunos meses; Gastos futuros toma el monto planeado en ese tiempo." />
      <Card className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-3">
          {data ? <PauseCircle className="mt-0.5 h-6 w-6 shrink-0 text-primary" aria-hidden="true" /> : <PlayCircle className="mt-0.5 h-6 w-6 shrink-0 text-muted" aria-hidden="true" />}
          <div className="min-w-0">
            {data ? (
              <>
                <p className="font-semibold">
                  Pausa activa: {data.months.length} {data.months.length === 1 ? "mes" : "meses"}, reanuda en {formatMonthName(data.resume_month)}
                </p>
                <p className="mt-1 text-sm text-muted">
                  {data.months.map(formatMonthName).join(", ")} · Presupuesto normal de Inversiones {formatMoney(data.normal_budget)}
                </p>
                {data.note && <p className="mt-1 break-words text-sm">{data.note}</p>}
              </>
            ) : (
              <>
                <p className="font-semibold">Sin pausa</p>
                <p className="mt-1 text-sm text-muted">Tus inversiones siguen con su presupuesto normal.</p>
              </>
            )}
          </div>
        </div>
        <button type="button" onClick={() => setEditing(true)} className={secondaryButton}>
          <Pencil className="h-4 w-4" aria-hidden="true" />
          {data ? "Editar" : "Configurar pausa"}
        </button>
      </Card>
      <MonthPreview />
      {editing && <PauseDialog pause={data} onClose={() => setEditing(false)} />}
    </div>
  );
}
