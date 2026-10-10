import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, Pencil, Plus, X } from "lucide-react";
import { createPersonalCategory, personalKeys, renamePersonalCategory, type PersonalCategory } from "../../api/personal";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { describePersonalError } from "./errors";

/** Add and rename her categories (no delete). Fixed categories never show the rename action. */
export function CategoriesDialog({ categories, onClose }: { categories: PersonalCategory[]; onClose: () => void }) {
  const qc = useQueryClient();
  const [newName, setNewName] = useState("");
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [draft, setDraft] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);

  // A rename changes the category name shown on every movement, so every personal query is refreshed.
  const refresh = () => qc.invalidateQueries({ queryKey: personalKeys.all });
  const create = useMutation({
    mutationFn: (name: string) => createPersonalCategory(name),
    onSuccess: async () => {
      setNewName("");
      await refresh();
    },
  });
  const rename = useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) => renamePersonalCategory(id, name),
    onSuccess: async () => {
      setRenamingId(null);
      await refresh();
    },
  });

  function onCreate(e: FormEvent) {
    e.preventDefault();
    rename.reset();
    if (newName.trim() === "") {
      setNameError("Escribe el nombre de la categoría.");
      return;
    }
    setNameError(null);
    create.mutate(newName.trim());
  }

  function onRename(e: FormEvent, id: number) {
    e.preventDefault();
    create.reset();
    if (draft.trim() === "") {
      setNameError("Escribe el nombre de la categoría.");
      return;
    }
    setNameError(null);
    rename.mutate({ id, name: draft.trim() });
  }

  const failure = create.error ?? rename.error;

  return (
    <Modal title="Mis categorías de gasto" onClose={onClose}>
      <div className="space-y-5">
        <form noValidate onSubmit={onCreate} className="space-y-3">
          <TextField label="Nueva categoría" value={newName} onChange={(e) => setNewName(e.target.value)} error={renamingId === null ? (nameError ?? undefined) : undefined} autoComplete="off" />
          <button type="submit" disabled={create.isPending} className={primaryButton}>
            {create.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Plus className="h-4 w-4" aria-hidden="true" />}
            Agregar categoría
          </button>
        </form>

        {failure && <ErrorBanner>{describePersonalError(failure, "la categoría")}</ErrorBanner>}

        <ul className="divide-y divide-border rounded-xl border border-border">
          {categories.map((c) => (
            <li key={c.id} className="p-3">
              {renamingId === c.id ? (
                <form noValidate onSubmit={(e) => onRename(e, c.id)} aria-label={`Renombrar ${c.name}`} className="space-y-3">
                  <TextField label="Nuevo nombre" value={draft} onChange={(e) => setDraft(e.target.value)} error={nameError ?? undefined} autoComplete="off" />
                  <div className="flex flex-wrap gap-2">
                    <button type="submit" disabled={rename.isPending} className={primaryButton}>
                      {rename.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Check className="h-4 w-4" aria-hidden="true" />}
                      Guardar nombre
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setRenamingId(null);
                        setNameError(null);
                        rename.reset();
                      }}
                      className={secondaryButton}
                    >
                      <X className="h-4 w-4" aria-hidden="true" />
                      Cancelar
                    </button>
                  </div>
                </form>
              ) : (
                <div className="flex items-center justify-between gap-3">
                  <span className="min-w-0 break-words font-medium">{c.name}</span>
                  {!c.fixed && (
                    <button
                      type="button"
                      aria-label={`Renombrar ${c.name}`}
                      onClick={() => {
                        create.reset();
                        rename.reset();
                        setNameError(null);
                        setDraft(c.name);
                        setRenamingId(c.id);
                      }}
                      className={secondaryButton}
                    >
                      <Pencil className="h-4 w-4" aria-hidden="true" />
                      Renombrar
                    </button>
                  )}
                </div>
              )}
            </li>
          ))}
        </ul>
        <button type="button" onClick={onClose} className={secondaryButton}>
          Cerrar
        </button>
      </div>
    </Modal>
  );
}
