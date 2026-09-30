import { useState } from "react";
import { updateCategories, type Category } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { Card, SectionForm, fieldGrid, useDraft, useSave } from "./ui";
import { fromCategoryDraft, toCategoryDraft, validateCategories, type CategoryDraft, type Errors } from "./validation";

export function CategoriesSection({ data }: { data: Category[] }) {
  const [draft, setDraft] = useDraft(data, (cats) => cats.map(toCategoryDraft));
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateCategories);

  const patch = (i: number, change: Partial<CategoryDraft>) => setDraft(draft.map((c, j) => (j === i ? { ...c, ...change } : c)));

  function onSubmit() {
    const found = validateCategories(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(draft.map(fromCategoryDraft));
    return true;
  }

  return (
    <SectionForm
      title="Categorías y presupuestos"
      description="Presupuesto mensual por categoría en MXN. Déjalo vacío si la categoría no tiene presupuesto."
      save={save}
      onSubmit={onSubmit}
    >
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
                  onChange={(e) => patch(i, { budget: e.target.value })}
                  error={errors[`${i}.budget`]}
                />
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
