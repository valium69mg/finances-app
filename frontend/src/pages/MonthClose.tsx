import { useState } from "react";
import { CheckCircle2 } from "lucide-react";
import type { MonthClose as StoredClose } from "../api/monthClose";
import { TextField } from "../components/AuthCard";
import { History } from "./monthclose/History";
import { PreviewPanel } from "./monthclose/PreviewPanel";
import { periodLabel } from "./taxfiling/labels";
import { useCyclePeriod } from "./useCycle";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";

/** Cierre de mes: a preview of a month and its stored snapshots. Replaces /cierre-mes. */
export function MonthClose() {
  const { month: period, setMonth: setPeriod, currentMonth, rangeHint } = useCyclePeriod("previous");
  const [selected, setSelected] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  function onClosed(close: StoredClose) {
    setNotice(`Cierre de ${periodLabel(close.period)} guardado. Lo encuentras en el historial.`);
    setSelected(close.period);
  }

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/cierre-de-mes")}>Cierre de mes</PageTitle>
      <p className="mt-1 text-sm text-muted">
        Revisa cómo te fue en un mes: gastos contra presupuesto, dinero disponible y avance del fondo de emergencia. Al cerrarlo se guarda una foto fija de esas cifras.
      </p>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="w-full min-w-0 max-w-full sm:w-56">
          <TextField
            label="Mes a cerrar"
            type="month"
            hint={rangeHint || undefined}
            value={period ?? ""}
            onChange={(e) => {
              if (!e.target.value) return;
              setPeriod(e.target.value);
              setNotice(null);
            }}
          />
        </div>
        <div className="mt-6">
          {period === null ? (
            <p role="status" className="text-sm text-muted">
              Cargando periodo…
            </p>
          ) : (
            <PreviewPanel
              key={period}
              period={period}
              currentMonth={currentMonth}
              onClosed={onClosed}
              onOpenStored={setSelected}
            />
          )}
        </div>
      </div>

      {notice && (
        <p role="status" className="mt-6 flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span className="min-w-0 break-words">{notice}</span>
        </p>
      )}

      <div id="month-close-history" className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <h2 className="mb-4 text-lg font-semibold tracking-tight">Cierres guardados</h2>
        <History
          selected={selected}
          onSelect={setSelected}
          onDeleted={(p) => {
            setSelected((cur) => (cur === p ? null : cur));
            setNotice(`Cierre de ${periodLabel(p)} eliminado. Puedes generarlo de nuevo.`);
          }}
        />
      </div>
    </section>
  );
}
