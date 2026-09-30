import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { updateBrackets, type Bracket } from "../../api/settings";
import { FieldError, TextField } from "../../components/AuthCard";
import { Card, SectionForm, secondaryButton, useDraft, useSave } from "./ui";
import { validateBrackets, type Errors } from "./validation";

export function BracketsSection({ data }: { data: Bracket[] }) {
  const [draft, setDraft] = useDraft(data, (d) => d);
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateBrackets);

  const patch = (i: number, key: keyof Bracket, value: string) => setDraft(draft.map((b, j) => (j === i ? { ...b, [key]: value } : b)));

  function onSubmit() {
    const found = validateBrackets(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(draft);
    return true;
  }

  return (
    <SectionForm
      title="Rangos de RESICO"
      description="Límite superior mensual de ingresos (MXN) y tasa de ISR de cada rango, de menor a mayor."
      save={save}
      onSubmit={onSubmit}
      extraActions={
        <button type="button" onClick={() => setDraft([...draft, { upper: "", rate: "" }])} className={secondaryButton}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          Agregar rango
        </button>
      }
    >
      {errors.rows && <FieldError id="brackets-rows">{errors.rows}</FieldError>}
      <ul className="space-y-3">
        {draft.map((b, i) => (
          <li key={i}>
            <Card className="flex flex-col gap-4 sm:flex-row sm:items-start">
              <div className="grid min-w-0 flex-1 gap-4 sm:grid-cols-2">
                <TextField label={`Límite superior (rango ${i + 1})`} inputMode="decimal" value={b.upper} onChange={(e) => patch(i, "upper", e.target.value)} error={errors[`${i}.upper`]} />
                <TextField
                  label={`Tasa de ISR (rango ${i + 1})`}
                  hint="Como fracción: 0.0125 equivale a 1.25 %."
                  inputMode="decimal"
                  value={b.rate}
                  onChange={(e) => patch(i, "rate", e.target.value)}
                  error={errors[`${i}.rate`]}
                />
              </div>
              <button
                type="button"
                onClick={() => setDraft(draft.filter((_, j) => j !== i))}
                aria-label={`Quitar rango ${i + 1}`}
                className="focus-ring flex h-11 w-11 shrink-0 items-center justify-center self-end rounded-lg text-muted transition-colors duration-200 hover:bg-destructive/10 hover:text-destructive sm:mt-7 sm:self-start"
              >
                <Trash2 className="h-5 w-5" aria-hidden="true" />
              </button>
            </Card>
          </li>
        ))}
      </ul>
    </SectionForm>
  );
}
