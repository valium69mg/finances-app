import { useState } from "react";
import { updateGeneral, type General } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { Checkbox, SectionForm, fieldGrid, useDraft, useSave } from "./ui";
import { validateGeneral, type Errors } from "./validation";

const FIELDS = [
  { key: "salary_usd", label: "Salario mensual (USD)" },
  { key: "fx_rate_applied", label: "Tipo de cambio aplicado (MXN por USD)" },
  { key: "morse_fee_rate", label: "Comisión de Morse (tasa)", hint: "Como fracción: 0.01 equivale a 1 %." },
  { key: "emergency_months", label: "Meses de fondo de emergencia" },
  { key: "extra_income_estimate_mxn", label: "Ingreso extra estimado (MXN)" },
] as const;

export function GeneralSection({ data }: { data: General }) {
  const [draft, setDraft] = useDraft(data, (d) => d);
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateGeneral);

  function onSubmit() {
    const found = validateGeneral(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(draft);
    return true;
  }

  return (
    <SectionForm title="General" description="Ingresos, tipo de cambio y reglas base de tu presupuesto." save={save} onSubmit={onSubmit}>
      <div className={fieldGrid}>
        {FIELDS.map((f) => (
          <TextField
            key={f.key}
            label={f.label}
            hint={"hint" in f ? f.hint : undefined}
            inputMode="decimal"
            value={draft[f.key]}
            onChange={(e) => setDraft({ ...draft, [f.key]: e.target.value })}
            error={errors[f.key]}
          />
        ))}
      </div>
      <Checkbox
        label="El presupuesto incluye el ingreso extra estimado"
        checked={draft.budget_includes_extra_income}
        onChange={(v) => setDraft({ ...draft, budget_includes_extra_income: v })}
      />

      {Object.keys(draft.extra_income_split).length > 0 && (
        <fieldset className="space-y-3">
          <legend className="text-sm font-semibold">Reparto del ingreso extra</legend>
          <div className={fieldGrid}>
            {Object.entries(draft.extra_income_split).map(([key, value]) => (
              <TextField
                key={key}
                label={`Reparto: ${key}`}
                inputMode="decimal"
                value={value}
                onChange={(e) => setDraft({ ...draft, extra_income_split: { ...draft.extra_income_split, [key]: e.target.value } })}
                error={errors[`split.${key}`]}
              />
            ))}
          </div>
        </fieldset>
      )}

      {draft.investment_allocation.length > 0 && (
        <fieldset className="space-y-3">
          <legend className="text-sm font-semibold">Distribución de inversiones</legend>
          <div className={fieldGrid}>
            {draft.investment_allocation.map((w, i) => (
              <TextField
                key={w.key}
                label={`Inversión: ${w.key}`}
                inputMode="decimal"
                value={w.value}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    investment_allocation: draft.investment_allocation.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)),
                  })
                }
                error={errors[`alloc.${i}`]}
              />
            ))}
          </div>
        </fieldset>
      )}
    </SectionForm>
  );
}
