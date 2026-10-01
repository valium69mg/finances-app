import { useState } from "react";
import { updateCategories, type Category, type General } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { amountToPercent, baseIncome, distribution, isBudgetKind, percentToAmount } from "../../lib/planning";
import { DistributionBar } from "./DistributionBar";
import { PercentField } from "./PercentField";
import { Card, ErrorBanner, SectionForm, fieldGrid, useDraft, useSave } from "./ui";
import { fromCategoryDraft, toCategoryDraft, validateCategories, type CategoryDraft, type Errors } from "./validation";

export function CategoriesSection({ data, general }: { data: Category[]; general: General }) {
  const [draft, setDraft] = useDraft(data, (cats) => cats.map(toCategoryDraft));
  const [errors, setErrors] = useState<Errors>({});
  /** Percentage text while it is being typed, by row; the rest of the time it is derived from the amount. */
  const [typing, setTyping] = useState<Record<number, string>>({});
  const save = useSave(updateCategories);

  // Base monthly income (estimate for planning); null disables every percentage feature.
  const base = baseIncome(general.salary_usd, general.fx_rate_applied);
  const dist = base ? distribution(draft.filter((c) => isBudgetKind(c.kind)), base) : null;

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
      notice={errors["total"] && <ErrorBanner>{errors["total"]}</ErrorBanner>}
    >
      {dist && base && <DistributionBar distribution={dist} salaryUsd={general.salary_usd} fxRate={general.fx_rate_applied} />}
      {draft.length === 0 && <p className="text-sm text-muted">Aún no hay categorías configuradas.</p>}
      <ul className="space-y-3">
        {draft.map((c, i) => (
          <li key={c.name}>
            <Card>
              <div className="mb-3 flex flex-wrap items-center gap-2">
                <h3 className="min-w-0 break-words font-semibold">{c.name}</h3>
                <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted">{c.kind}</span>
              </div>
              <div className={fieldGrid}>
                <TextField
                  label={`Presupuesto de ${c.name}`}
                  inputMode="decimal"
                  value={c.budget}
                  onChange={(e) => onAmount(i, e.target.value)}
                  error={errors[`${i}.budget`]}
                />
                {base && isBudgetKind(c.kind) && (
                  <PercentField
                    label={`Porcentaje del ingreso base de ${c.name} (%)`}
                    value={typing[i] ?? (amountToPercent(c.budget, base) ?? "")}
                    onChange={(e) => onPercent(i, e.target.value)}
                    onBlur={() => setTyping(({ [i]: _drop, ...rest }) => rest)}
                  />
                )}
                <TextField label={`Incluye (${c.name})`} value={c.includes} onChange={(e) => patch(i, { includes: e.target.value })} />
                <div className="sm:col-span-2">
                  <TextField
                    label={`Palabras clave (${c.name})`}
                    hint="Separadas por comas."
                    value={c.keywords}
                    onChange={(e) => patch(i, { keywords: e.target.value })}
                  />
                </div>
              </div>
            </Card>
          </li>
        ))}
      </ul>
    </SectionForm>
  );
}
