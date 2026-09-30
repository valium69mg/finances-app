import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Trash2 } from "lucide-react";
import { deletePause, getMonthBudgets, settingsKeys, updatePause, type Pause } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { Card, ErrorBanner, SectionForm, dangerButton, describeSaveError, fieldGrid, secondaryButton, useDraft, useSave } from "./ui";
import { isMonth, fromPauseDraft, parseMonths, toPauseDraft, validatePause, type Errors } from "./validation";

function currentMonth(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

function MonthPreview() {
  const [month, setMonth] = useState(currentMonth);
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

export function PauseSection({ data }: { data: Pause | null }) {
  const [draft, setDraft] = useDraft(data, toPauseDraft);
  const [errors, setErrors] = useState<Errors>({});
  const [confirming, setConfirming] = useState(false);
  const save = useSave(updatePause);
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: deletePause,
    onSuccess: () => {
      setConfirming(false);
      return qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });

  const months = parseMonths(draft.monthsText).filter(isMonth);

  function onSubmit() {
    const found = validatePause(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(fromPauseDraft(draft));
    return true;
  }

  return (
    <div className="space-y-8">
      <SectionForm
        title="Pausa de inversiones"
        description={
          data
            ? "Durante los meses en pausa, Inversiones queda en 0 y Gastos futuros toma el monto planeado. Desde el mes de reanudación vuelve el presupuesto normal."
            : "No hay una pausa activa. Complétalo para pausar tus inversiones durante algunos meses."
        }
        save={save}
        onSubmit={onSubmit}
        submitLabel={data ? "Guardar pausa" : "Activar pausa"}
        extraActions={
          data && !confirming ? (
            <button type="button" onClick={() => setConfirming(true)} className={dangerButton}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
              Cancelar pausa
            </button>
          ) : undefined
        }
      >
        <div className={fieldGrid}>
          <TextField
            label="Meses en pausa"
            hint="Formato AAAA-MM, separados por comas."
            placeholder="2026-10, 2026-11"
            value={draft.monthsText}
            onChange={(e) => setDraft({ ...draft, monthsText: e.target.value })}
            error={errors.months}
          />
          <TextField
            label="Mes de reanudación"
            placeholder="2027-02"
            value={draft.resume_month}
            onChange={(e) => setDraft({ ...draft, resume_month: e.target.value })}
            error={errors.resume_month}
          />
          <TextField
            label="Presupuesto normal de Inversiones (MXN)"
            inputMode="decimal"
            value={draft.normal_budget}
            onChange={(e) => setDraft({ ...draft, normal_budget: e.target.value })}
            error={errors.normal_budget}
          />
          <TextField label="Nota" value={draft.note} onChange={(e) => setDraft({ ...draft, note: e.target.value })} />
        </div>
        {months.length > 0 && (
          <fieldset className="space-y-3">
            <legend className="text-sm font-semibold">Plan de Gastos futuros por mes (MXN)</legend>
            <div className={fieldGrid}>
              {months.map((m) => (
                <TextField
                  key={m}
                  label={`Gastos futuros de ${m}`}
                  inputMode="decimal"
                  value={draft.plan[m] ?? ""}
                  onChange={(e) => setDraft({ ...draft, plan: { ...draft.plan, [m]: e.target.value } })}
                  error={errors[`plan.${m}`]}
                />
              ))}
            </div>
          </fieldset>
        )}
        {data && confirming && (
          <div role="group" aria-label="Confirmar cancelación de la pausa" className="space-y-3 rounded-lg border border-destructive/40 p-4">
            <p className="text-sm">¿Cancelar la pausa? Inversiones volverá a su presupuesto normal en todos los meses.</p>
            {remove.isError && <ErrorBanner>{describeSaveError(remove.error)}</ErrorBanner>}
            <div className="flex flex-wrap gap-3">
              <button type="button" onClick={() => remove.mutate()} disabled={remove.isPending} className={dangerButton}>
                {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                Sí, cancelar pausa
              </button>
              <button type="button" onClick={() => setConfirming(false)} className={secondaryButton}>
                Conservar pausa
              </button>
            </div>
          </div>
        )}
      </SectionForm>
      <MonthPreview />
    </div>
  );
}
