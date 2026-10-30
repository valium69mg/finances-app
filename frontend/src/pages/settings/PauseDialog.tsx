import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Trash2 } from "lucide-react";
import { deletePause, settingsKeys, updatePause, type Pause } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { EditDialog } from "./EditDialog";
import { ErrorBanner, dangerButton, describeSaveError, fieldGrid, secondaryButton, useSave } from "./ui";
import { fromPauseDraft, isMonth, parseMonths, toPauseDraft, validatePause, type Errors } from "./validation";

/** Form of the investment pause (null = no pause yet). A saved pause can also be cancelled, after a confirmation. */
export function PauseDialog({ pause, onClose }: { pause: Pause | null; onClose: () => void }) {
  const [draft, setDraft] = useState(() => toPauseDraft(pause));
  const [errors, setErrors] = useState<Errors>({});
  const [confirming, setConfirming] = useState(false);
  const save = useSave(updatePause);
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: deletePause,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: settingsKeys.all });
      onClose();
    },
  });

  const months = parseMonths(draft.monthsText).filter(isMonth);

  function onSubmit() {
    const found = validatePause(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(fromPauseDraft(draft), { onSuccess: onClose });
    return true;
  }

  return (
    <EditDialog
      title={pause ? "Editar pausa de inversiones" : "Configurar pausa de inversiones"}
      editing={pause !== null}
      onClose={onClose}
      save={save}
      onSubmit={onSubmit}
      submitLabel={pause ? "Guardar pausa" : "Activar pausa"}
      extraActions={
        pause && !confirming ? (
          <button type="button" onClick={() => setConfirming(true)} className={dangerButton}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            Cancelar pausa
          </button>
        ) : undefined
      }
    >
      <p className="text-sm text-muted">
        Durante los meses en pausa, Inversiones queda en 0 y Gastos futuros toma el monto planeado. Desde el mes de reanudación vuelve el presupuesto normal.
      </p>
      <div className={fieldGrid}>
        <TextField
          label="Meses en pausa"
          hint="Formato AAAA-MM, separados por comas."
          placeholder="2026-10, 2026-11"
          value={draft.monthsText}
          onChange={(e) => setDraft({ ...draft, monthsText: e.target.value })}
          error={errors.months}
        />
        <TextField label="Mes de reanudación" placeholder="2027-02" value={draft.resume_month} onChange={(e) => setDraft({ ...draft, resume_month: e.target.value })} error={errors.resume_month} />
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
      {pause && confirming && (
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
    </EditDialog>
  );
}
