import { useId, useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import type { PersonalCategory } from "../../api/personal";
import { createWishItem, updateWishItem, wishlistKeys, WISH_PRIORITIES, type WishItem } from "../../api/wishlist";
import { FieldError, TextField } from "../../components/AuthCard";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeWishError } from "./errors";
import { draftOf, emptyDraft, MAX_NAME_LENGTH, MAX_NOTE_LENGTH, toWishInput, validateWish, type WishDraft, type WishErrors } from "./form";
import { PRIORITY_NAME } from "./labels";

interface Props {
  /** Her Gasto categories. */
  categories: PersonalCategory[];
  /** When set, the form edits this item instead of creating one. Remount (key) to switch. */
  editing: WishItem | null;
  onSaved: (item: WishItem, wasEdit: boolean) => void;
  onCancel: () => void;
}

/** Create / edit form of a wishlist item (rendered inside a dialog that carries the title): name, estimated price, category, store link, priority and note. */
export function WishForm({ categories, editing, onSaved, onCancel }: Props) {
  const qc = useQueryClient();
  const noteId = useId();
  const noteErrorId = `${noteId}-error`;
  const [draft, setDraft] = useState<WishDraft>(() => (editing ? draftOf(editing) : emptyDraft(String(categories[0]?.id ?? ""))));
  const [errors, setErrors] = useState<WishErrors>({});
  const nameRef = useRef<HTMLInputElement>(null);
  const priceRef = useRef<HTMLInputElement>(null);
  const categoryRef = useRef<HTMLSelectElement>(null);
  const urlRef = useRef<HTMLInputElement>(null);
  const noteRef = useRef<HTMLTextAreaElement>(null);
  const set = (patch: Partial<WishDraft>) => setDraft((d) => ({ ...d, ...patch }));

  const save = useMutation({
    mutationFn: () => {
      const input = toWishInput(draft);
      return editing ? updateWishItem(editing.id, input) : createWishItem(input);
    },
    onSuccess: async (item) => {
      await qc.invalidateQueries({ queryKey: wishlistKeys.all });
      onSaved(item, editing !== null);
      if (!editing) setDraft(emptyDraft(String(categories[0]?.id ?? "")));
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    save.reset();
    const found = validateWish(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.name) nameRef.current?.focus();
      else if (found.price) priceRef.current?.focus();
      else if (found.category) categoryRef.current?.focus();
      else if (found.storeUrl) urlRef.current?.focus();
      else noteRef.current?.focus();
      return;
    }
    save.mutate();
  }

  // A category of the item that is no longer listed stays selectable while editing.
  const options = editing && !categories.some((c) => c.id === editing.category_id) ? [...categories, { id: editing.category_id, name: editing.category }] : categories;

  return (
    <form noValidate onSubmit={onSubmit} aria-label={editing ? `Editar ${editing.name}` : "Nuevo deseo"} className="space-y-5">
      <div className={fieldGrid}>
        <TextField label="Nombre" inputRef={nameRef} value={draft.name} onChange={(e) => set({ name: e.target.value })} error={errors.name} maxLength={MAX_NAME_LENGTH + 20} autoComplete="off" />
        <TextField
          label="Precio estimado (MXN)"
          inputMode="decimal"
          inputRef={priceRef}
          value={draft.price}
          onChange={(e) => set({ price: e.target.value })}
          error={errors.price}
          autoComplete="off"
        />
        <SelectField label="Categoría" selectRef={categoryRef} value={draft.categoryId} onChange={(e) => set({ categoryId: e.target.value })} error={errors.category}>
          {options.map((c) => (
            <option key={c.id} value={String(c.id)}>
              {c.name}
            </option>
          ))}
        </SelectField>
        <TextField
          label="Enlace de la tienda (opcional)"
          inputMode="url"
          inputRef={urlRef}
          value={draft.storeUrl}
          onChange={(e) => set({ storeUrl: e.target.value })}
          error={errors.storeUrl}
          autoComplete="off"
        />
        <SelectField label="Prioridad" value={draft.priority} onChange={(e) => set({ priority: e.target.value as WishDraft["priority"] })}>
          {WISH_PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {PRIORITY_NAME[p]}
            </option>
          ))}
        </SelectField>
      </div>
      <div>
        <label htmlFor={noteId} className="mb-1.5 block text-sm font-medium">
          Nota (opcional)
        </label>
        <textarea
          id={noteId}
          ref={noteRef}
          value={draft.note}
          onChange={(e) => set({ note: e.target.value })}
          rows={2}
          maxLength={MAX_NOTE_LENGTH + 50}
          aria-invalid={errors.note ? true : undefined}
          aria-describedby={errors.note ? noteErrorId : undefined}
          className="focus-ring block min-h-16 w-full rounded-lg border border-input bg-surface px-3 py-2 text-foreground transition-colors duration-200 aria-[invalid=true]:border-destructive"
        />
        {errors.note && <FieldError id={noteErrorId}>{errors.note}</FieldError>}
      </div>
      {save.isError && <ErrorBanner>{describeWishError(save.error)}</ErrorBanner>}
      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : editing ? (
            <Save className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Plus className="h-4 w-4" aria-hidden="true" />
          )}
          {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Agregar deseo"}
        </button>
        <button type="button" onClick={onCancel} className={secondaryButton}>
          Cancelar
        </button>
      </div>
    </form>
  );
}
