import { useState } from "react";
import { updateCategories, type Category } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { amountToPercent, isBudgetKind, percentToAmount } from "../../lib/planning";
import { SelectField } from "../expenses/SelectField";
import { EditDialog } from "./EditDialog";
import { PercentField } from "./PercentField";
import { categoryListWith } from "./summary";
import { ErrorBanner, fieldGrid, useSave } from "./ui";
import { KINDS, fromCategoryDraft, newCategoryDraft, toCategoryDraft, validateCategories, type CategoryDraft, type Errors } from "./validation";

interface Props {
  /** The saved list: it is replaced as a whole, with this one category changed or added. */
  saved: Category[];
  /** Position of the category being edited, or null to add a new one. */
  index: number | null;
  /** Base monthly income, or null when it is not available (no percentages then). */
  base: string | null;
  onClose: () => void;
}

export function CategoryDialog({ saved, index, base, onClose }: Props) {
  const adding = index === null;
  const [draft, setDraft] = useState<CategoryDraft>(() => (adding ? newCategoryDraft() : toCategoryDraft(saved[index])));
  const [errors, setErrors] = useState<Errors>({});
  /** Percentage text while it is being typed; the rest of the time it is derived from the amount. */
  const [typing, setTyping] = useState<string | null>(null);
  const save = useSave(updateCategories);

  const patch = (change: Partial<CategoryDraft>) => setDraft({ ...draft, ...change });
  const position = adding ? saved.length : index;

  function onPercent(value: string) {
    setTyping(value);
    const amount = base ? percentToAmount(value.trim(), base) : null;
    if (amount !== null) patch({ budget: amount });
  }

  function onSubmit() {
    const found = validateCategories(categoryListWith(saved, draft, index), base);
    // Only this category's own errors and the shared total are on screen.
    const own: Errors = Object.fromEntries(Object.entries(found).filter(([k]) => k === "total" || k.startsWith(`${position}.`)));
    setErrors(own);
    if (Object.keys(own).length > 0) return false;
    save.mutate(categoryListWith(saved, draft, index).map(fromCategoryDraft), { onSuccess: onClose });
    return true;
  }

  return (
    <EditDialog
      title={adding ? "Agregar categoría" : `Editar ${draft.name}`}
      editing={!adding}
      onClose={onClose}
      save={save}
      onSubmit={onSubmit}
      submitLabel={adding ? "Agregar" : "Guardar cambios"}
      notice={errors["total"] && <ErrorBanner>{errors["total"]}</ErrorBanner>}
    >
      {adding ? (
        <div className={fieldGrid}>
          <TextField label="Nombre" value={draft.name} onChange={(e) => patch({ name: e.target.value })} error={errors[`${position}.name`]} autoComplete="off" />
          <SelectField label="Tipo" value={draft.kind} onChange={(e) => patch({ kind: e.target.value })}>
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </SelectField>
        </div>
      ) : (
        <p className="flex flex-wrap items-center gap-2 text-sm text-muted">
          <span className="rounded-full border border-border px-2 py-0.5 text-xs">{draft.kind}</span>
          El nombre y el tipo no se pueden cambiar.
        </p>
      )}
      <div className={fieldGrid}>
        <TextField label="Presupuesto" inputMode="decimal" value={draft.budget} onChange={(e) => (setTyping(null), patch({ budget: e.target.value }))} error={errors[`${position}.budget`]} autoComplete="off" />
        {base && isBudgetKind(draft.kind) && (
          <PercentField
            label="Porcentaje del ingreso base (%)"
            value={typing ?? amountToPercent(draft.budget, base) ?? ""}
            onChange={(e) => onPercent(e.target.value)}
            onBlur={() => setTyping(null)}
          />
        )}
        <div className="sm:col-span-2">
          <TextField label="Incluye" value={draft.includes} onChange={(e) => patch({ includes: e.target.value })} />
        </div>
        <div className="sm:col-span-2">
          <TextField label="Palabras clave" hint="Separadas por comas." value={draft.keywords} onChange={(e) => patch({ keywords: e.target.value })} />
        </div>
      </div>
      <p className="text-sm text-muted">Presupuesto mensual en MXN. Déjalo vacío si la categoría no tiene presupuesto.</p>
    </EditDialog>
  );
}
