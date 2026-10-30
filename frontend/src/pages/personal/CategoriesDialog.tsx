import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Plus } from "lucide-react";
import { createPersonalCategory, personalKeys, renamePersonalCategory, type PersonalCategory } from "../../api/personal";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { EditDialog } from "../settings/EditDialog";
import { ErrorBanner, primaryButton, rowButton, secondaryButton } from "../settings/ui";
import { describePersonalError } from "./errors";

const NAME_REQUIRED = "Escribe el nombre de la categoría.";

/** The name form of one category: "Nueva categoría" (add) or "Renombrar" (edit). Closes itself when the save succeeds. */
function NameDialog({ category, onClose }: { category: PersonalCategory | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(category?.name ?? "");
  const [error, setError] = useState<string | null>(null);
  // A rename changes the category name shown on every movement, so every personal query is refreshed.
  const save = useMutation({
    mutationFn: (value: string) => (category ? renamePersonalCategory(category.id, value) : createPersonalCategory(value)),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: personalKeys.all });
      onClose();
    },
  });

  function onSubmit(): boolean {
    if (name.trim() === "") {
      setError(NAME_REQUIRED);
      return false;
    }
    setError(null);
    save.mutate(name.trim());
    return true;
  }

  return (
    <EditDialog
      title={category ? `Renombrar ${category.name}` : "Nueva categoría"}
      editing={category !== null}
      onClose={onClose}
      save={{ isPending: save.isPending, isError: false, error: null }}
      onSubmit={onSubmit}
      submitLabel={category ? "Guardar nombre" : "Agregar categoría"}
      notice={save.isError ? <ErrorBanner>{describePersonalError(save.error, "la categoría")}</ErrorBanner> : undefined}
    >
      <TextField label={category ? "Nuevo nombre" : "Nueva categoría"} value={name} onChange={(e) => setName(e.target.value)} error={error ?? undefined} autoComplete="off" />
    </EditDialog>
  );
}

/**
 * Her categories as a summary list: a row opens its rename dialog (fixed categories are plain rows, never editable)
 * and "Agregar categoría" opens the create dialog. No delete.
 */
export function CategoriesDialog({ categories, onClose }: { categories: PersonalCategory[]; onClose: () => void }) {
  const [form, setForm] = useState<{ category: PersonalCategory | null } | null>(null);

  return (
    <>
      <Modal title="Mis categorías de gasto" onClose={onClose}>
        <div className="space-y-5">
          <button type="button" onClick={() => setForm({ category: null })} className={primaryButton}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Agregar categoría
          </button>

          <ul className="space-y-2">
            {categories.map((c) => (
              <li key={c.id}>
                {c.fixed ? (
                  <div className="flex min-h-11 items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-3">
                    <span className="min-w-0 break-words font-medium">{c.name}</span>
                    <span className="shrink-0 text-xs text-muted">Fija</span>
                  </div>
                ) : (
                  <button type="button" aria-label={`Renombrar ${c.name}`} onClick={() => setForm({ category: c })} className={rowButton}>
                    <span className="min-w-0 flex-1 break-words font-medium">{c.name}</span>
                    <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
                  </button>
                )}
              </li>
            ))}
          </ul>
          <button type="button" onClick={onClose} className={secondaryButton}>
            Cerrar
          </button>
        </div>
      </Modal>
      {form && <NameDialog key={form.category?.id ?? "new"} category={form.category} onClose={() => setForm(null)} />}
    </>
  );
}
