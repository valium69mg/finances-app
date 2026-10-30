import { useState } from "react";
import { updateGeneral, type General } from "../../api/settings";
import { FieldError, TextField } from "../../components/AuthCard";
import { isHundred } from "../../lib/planning";
import { SelectField } from "../expenses/SelectField";
import { todayISO } from "../expenses/money";
import { EditDialog } from "./EditDialog";
import { PercentField } from "./PercentField";
import { currentPeriodText } from "./summary";
import { Checkbox, fieldGrid, useSave } from "./ui";
import {
  SPLIT_DESTINATIONS,
  allocationTotal,
  fromGeneralDraft,
  splitDestinationsTotal,
  toGeneralDraft,
  validateGeneralBlock,
  type Errors,
  type GeneralBlock,
} from "./validation";

/** Spanish names of the extra-income split keys. */
export const SPLIT_LABELS: Record<string, string> = {
  sat_reserve_rate: "Reserva SAT",
  fondo_emergencia: "Fondo de emergencia",
  inversiones: "Inversiones",
  aguinaldo_vacaciones: "Aguinaldo y vacaciones",
};

const SPLIT_KEYS = ["sat_reserve_rate", ...SPLIT_DESTINATIONS];
export const orderedSplitKeys = (split: Record<string, string>) => [...SPLIT_KEYS.filter((k) => k in split), ...Object.keys(split).filter((k) => !SPLIT_KEYS.includes(k))];

export const BLOCK_TITLES: Record<GeneralBlock, string> = {
  income: "Ingreso base",
  extra: "Ingreso extra y fondo de emergencia",
  period: "Periodo",
  split: "Reparto del ingreso extra",
  alloc: "Distribución de inversiones",
};

/** Live "Suma: X %" line; it turns destructive (and says so) while the parts do not add up to 100 %. */
function SumLine({ total, label }: { total: string | null; label: string }) {
  if (total === null) return null;
  const ok = isHundred(total);
  return (
    <p aria-live="polite" className={`text-sm ${ok ? "text-muted" : "font-medium text-destructive"}`}>
      {label}: {total} %{ok ? "" : " (debe sumar 100 %)"}
    </p>
  );
}

export function GeneralDialog({ data, block, onClose }: { data: General; block: GeneralBlock; onClose: () => void }) {
  const [draft, setDraft] = useState(() => toGeneralDraft(data));
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateGeneral);

  function onSubmit() {
    const found = validateGeneralBlock(draft, block);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    // The rest of the General object goes along unchanged: the API replaces the whole section.
    save.mutate(fromGeneralDraft(draft), { onSuccess: onClose });
    return true;
  }

  const text = (key: "salary_usd" | "fx_rate_applied" | "morse_fee_rate" | "emergency_months" | "extra_income_estimate_mxn", label: string, hint?: string) => (
    <TextField label={label} hint={hint} inputMode="decimal" value={draft[key]} onChange={(e) => setDraft({ ...draft, [key]: e.target.value })} error={errors[key]} autoComplete="off" />
  );

  return (
    <EditDialog title={`Editar ${BLOCK_TITLES[block].toLowerCase()}`} onClose={onClose} save={save} onSubmit={onSubmit}>
      {block === "income" && (
        <div className={fieldGrid}>
          {text("salary_usd", "Salario mensual (USD)")}
          {text("fx_rate_applied", "Tipo de cambio aplicado (MXN por USD)")}
          {text("morse_fee_rate", "Comisión de Morse (tasa)", "Como fracción: 0.01 equivale a 1 %.")}
        </div>
      )}

      {block === "extra" && (
        <>
          <div className={fieldGrid}>
            {text("emergency_months", "Meses de fondo de emergencia")}
            {text("extra_income_estimate_mxn", "Ingreso extra estimado (MXN)")}
          </div>
          <Checkbox
            label="El presupuesto incluye el ingreso extra estimado"
            checked={draft.budget_includes_extra_income}
            onChange={(v) => setDraft({ ...draft, budget_includes_extra_income: v })}
          />
        </>
      )}

      {block === "period" && (
        <>
          <SelectField
            label="Inicio del periodo"
            hint="Con “Último día del mes”, el periodo de octubre va del 30 de septiembre al 30 de octubre; con “Último día hábil”, el periodo empieza el último día de lunes a viernes del mes. Gastos, ingresos, ahorros y cierre de mes usan este periodo; las facturas y la declaración siguen por mes calendario."
            value={String(draft.cycle_start_day)}
            onChange={(e) => setDraft({ ...draft, cycle_start_day: Number(e.target.value) })}
            error={errors["cycle_start_day"]}
          >
            <option value="0">Mes calendario</option>
            <option value="31">Último día del mes</option>
            <option value="32">Último día hábil (lun–vie)</option>
            {![0, 31, 32].includes(draft.cycle_start_day) && <option value={draft.cycle_start_day}>{`Día ${draft.cycle_start_day} (configurado)`}</option>}
          </SelectField>
          <p aria-live="polite" className="text-sm font-medium">
            {currentPeriodText(draft.cycle_start_day, todayISO())}
          </p>
        </>
      )}

      {block === "split" && (
        <fieldset className="space-y-3">
          <legend className="sr-only">Reparto del ingreso extra</legend>
          <p className="text-sm text-muted">La reserva SAT se aparta primero; lo demás se reparte entre fondo de emergencia, inversiones y aguinaldo y vacaciones, que deben sumar 100 %.</p>
          <div className={fieldGrid}>
            {orderedSplitKeys(draft.extra_income_split).map((key) => (
              <PercentField
                key={key}
                label={`${SPLIT_LABELS[key] ?? key} (%)`}
                value={draft.extra_income_split[key]}
                onChange={(e) => setDraft({ ...draft, extra_income_split: { ...draft.extra_income_split, [key]: e.target.value } })}
                error={errors[`split.${key}`]}
                aria-invalid={errors["split"] && key === SPLIT_DESTINATIONS[0] ? true : undefined}
              />
            ))}
          </div>
          <SumLine total={splitDestinationsTotal(draft.extra_income_split)} label="Suma de los tres destinos" />
          {errors["split"] && <FieldError id="split-sum-error">{errors["split"]}</FieldError>}
        </fieldset>
      )}

      {block === "alloc" && (
        <fieldset className="space-y-3">
          <legend className="sr-only">Distribución de inversiones</legend>
          <div className={fieldGrid}>
            {draft.investment_allocation.map((w, i) => (
              <PercentField
                key={w.key}
                label={`Inversión: ${w.key} (%)`}
                value={w.value}
                onChange={(e) =>
                  setDraft({ ...draft, investment_allocation: draft.investment_allocation.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)) })
                }
                error={errors[`alloc.${i}`]}
                aria-invalid={errors["alloc"] && i === 0 ? true : undefined}
              />
            ))}
          </div>
          <SumLine total={allocationTotal(draft.investment_allocation)} label="Suma de la distribución" />
          {errors["alloc"] && <FieldError id="alloc-sum-error">{errors["alloc"]}</FieldError>}
        </fieldset>
      )}
    </EditDialog>
  );
}
