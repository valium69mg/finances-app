import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { updateCategories, type Category, type General } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { amountToPercent, baseIncome, distribution, isBudgetKind, percentToAmount } from "../../lib/planning";
import { DistributionBar } from "./DistributionBar";
import { PercentField } from "./PercentField";
import { SelectField } from "../expenses/SelectField";
import { Card, ErrorBanner, SectionForm, fieldGrid, secondaryButton, useDraft, useSave } from "./ui";
import { KINDS, fromCategoryDraft, newCategoryDraft, toCategoryDraft, validateCategories, type CategoryDraft, type Errors } from "./validation";

export function CategoriesSection({ data, general }: { data: Category[]; general: General }) {
  const [draft, setDraft] = useDraft(data, (cats) => cats.map(toCategoryDraft));
  const [errors, setErrors] = useState<Errors>({});
  /** Percentage text while it is being typed, by row; the rest of the time it is derived from the amount. */
  const [typing, setTyping] = useState<Record<number, string>>({});
  const save = useSave(updateCategories);

  // Base monthly income (estimate for planning); null disables every percentage feature.
  const base = baseIncome(general.salary_usd, general.fx_rate_applied);
  const dist = base
    ? distribution(
        draft.filter((c) => isBudgetKind(c.kind)),
        base,
      )
    : null;

  const patch = (i: number, change: Partial<CategoryDraft>) => setDraft(draft.map((c, j) => (j === i ? { ...c, ...change } : c)));

  function onAmount(i: number, value: string) {
    setTyping(({ [i]: _drop, ...rest }) => rest);
    patch(i, { budget: value });
  }

  function onPercent(i: number, value: string) {
    setTyping({ ...typing, [i]: value });
    const amount = base ? percentToAmount(value.trim(), base) : null;
    if (amount !== null) patch(i, { budget: amount });
  }

  function addCategory() {
    setDraft([...draft, newCategoryDraft()]);
  }

  function removeCategory(i: number) {
    setDraft(draft.filter((_, j) => j !== i));
    setErrors({});
    setTyping({});
  }

  function onSubmit() {
    const found = validateCategories(draft, base);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(draft.map(fromCategoryDraft));
    return true;
  }

  return (
    <SectionForm
      title="Categorías y presupuestos"
      description="Presupuesto mensual por categoría en MXN. Déjalo vacío si la categoría no tiene presupuesto. Si hay ingreso base, también puedes escribir el porcentaje y el monto se calcula."
      save={save}
      onSubmit={onSubmit}
      extraActions={
        <button type="button" onClick={addCategory} className={secondaryButton}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          Agregar categoría
        </button>
      }
      notice={errors["total"] && <ErrorBanner>{errors["total"]}</ErrorBanner>}
    >
      {dist && base && <DistributionBar distribution={dist} salaryUsd={general.salary_usd} fxRate={general.fx_rate_applied} />}
      {draft.length === 0 && <p className="text-sm text-muted">Aún no hay categorías configuradas.</p>}
      <ul className="space-y-3">
        {draft.map((c, i) => {
          const label = c.name.trim() || "nueva categoría";
          return (
            <li key={c.uid ?? `saved-${c.name}`}>
              <Card>
                {c.isNew ? (
                  <div className="mb-3 flex items-start gap-2">
                    <div className={`${fieldGrid} min-w-0 flex-1`}>
                      <TextField label="Nombre" value={c.name} onChange={(e) => patch(i, { name: e.target.value })} error={errors[`${i}.name`]} />
                      <SelectField label="Tipo" value={c.kind} onChange={(e) => patch(i, { kind: e.target.value })}>
                        {KINDS.map((k) => (
                          <option key={k} value={k}>
                            {k}
                          </option>
                        ))}
                      </SelectField>
                    </div>
                    <button
                      type="button"
                      onClick={() => removeCategory(i)}
                      aria-label={`Quitar ${label}`}
                      className="focus-ring flex h-11 shrink-0 items-center justify-center gap-1 self-start rounded-lg px-3 text-sm text-muted transition-colors duration-200 hover:bg-destructive/10 hover:text-destructive sm:mt-7"
                    >
                      <Trash2 className="h-5 w-5" aria-hidden="true" />
                      Quitar
                    </button>
                  </div>
                ) : (
                  <div className="mb-3 flex flex-wrap items-center gap-2">
                    <h3 className="min-w-0 break-words font-semibold">{c.name}</h3>
                    <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted">{c.kind}</span>
                  </div>
                )}
                <div className={fieldGrid}>
                  <TextField
                    label={`Presupuesto de ${label}`}
                    inputMode="decimal"
                    value={c.budget}
                    onChange={(e) => onAmount(i, e.target.value)}
                    error={errors[`${i}.budget`]}
                  />
                  {base && isBudgetKind(c.kind) && (
                    <PercentField
                      label={`Porcentaje del ingreso base de ${label} (%)`}
                      value={typing[i] ?? amountToPercent(c.budget, base) ?? ""}
                      onChange={(e) => onPercent(i, e.target.value)}
                      onBlur={() => setTyping(({ [i]: _drop, ...rest }) => rest)}
                    />
                  )}
                  <TextField label={`Incluye (${label})`} value={c.includes} onChange={(e) => patch(i, { includes: e.target.value })} />
                  <div className="sm:col-span-2">
                    <TextField
                      label={`Palabras clave (${label})`}
                      hint="Separadas por comas."
                      value={c.keywords}
                      onChange={(e) => patch(i, { keywords: e.target.value })}
                    />
                  </div>
                </div>
              </Card>
            </li>
          );
        })}
      </ul>
    </SectionForm>
  );
}
