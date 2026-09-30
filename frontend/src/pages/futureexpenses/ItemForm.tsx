import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import { createFutureExpense, updateFutureExpense, type FutureExpenseItem } from "../../api/futureExpenses";
import { TextField } from "../../components/AuthCard";
import { useFocusOnEdit } from "../../hooks/useFocusOnEdit";
import { ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeFutureError } from "./errors";
import { draftOf, emptyDraft, toItemInput, validateItem, type ItemDraft, type ItemErrors } from "./form";
import { useInvalidateFuture } from "./useInvalidate";

interface Props {
  /** When set, the form edits this item instead of creating one. Remount (key) to switch. */
  editing: FutureExpenseItem | null;
  onSaved: (item: FutureExpenseItem, wasEdit: boolean) => void;
  onCancelEdit: () => void;
}

/** Create / edit form of a future expense: name, target amount (MXN) and due date. */
export function ItemForm({ editing, onSaved, onCancelEdit }: Props) {
  const invalidate = useInvalidateFuture();
  const [draft, setDraft] = useState<ItemDraft>(() => (editing ? draftOf(editing) : emptyDraft()));
  const [errors, setErrors] = useState<ItemErrors>({});
  const formRef = useFocusOnEdit<HTMLFormElement>(editing !== null);
  const nameRef = useRef<HTMLInputElement>(null);
  const targetRef = useRef<HTMLInputElement>(null);
  const dateRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<ItemDraft>) => setDraft((d) => ({ ...d, ...patch }));

  const save = useMutation({
    mutationFn: () => {
      const input = toItemInput(draft);
      return editing ? updateFutureExpense(editing.id, input) : createFutureExpense(input);
    },
    onSuccess: async (item) => {
      await invalidate();
      onSaved(item, editing !== null);
      if (!editing) setDraft(emptyDraft());
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    save.reset();
    const found = validateItem(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.name) nameRef.current?.focus();
      else if (found.target) targetRef.current?.focus();
      else dateRef.current?.focus();
      return;
    }
    save.mutate();
  }

  return (
    <form ref={formRef} noValidate onSubmit={onSubmit} aria-labelledby="future-form-title" className="space-y-5">
      <h3 id="future-form-title" className="text-base font-semibold tracking-tight">
        {editing ? `Editar ${editing.name}` : "Nuevo gasto futuro"}
      </h3>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <TextField label="Nombre" inputRef={nameRef} value={draft.name} onChange={(e) => set({ name: e.target.value })} error={errors.name} autoComplete="off" />
        <TextField
          label="Monto a juntar (MXN)"
          inputMode="decimal"
          inputRef={targetRef}
          value={draft.target}
          onChange={(e) => set({ target: e.target.value })}
          error={errors.target}
          autoComplete="off"
        />
        <TextField label="Fecha de vencimiento" type="date" inputRef={dateRef} value={draft.dueDate} onChange={(e) => set({ dueDate: e.target.value })} error={errors.date} />
      </div>
      {save.isError && <ErrorBanner>{describeFutureError(save.error)}</ErrorBanner>}
      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : editing ? (
            <Save className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Plus className="h-4 w-4" aria-hidden="true" />
          )}
          {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Agregar gasto futuro"}
        </button>
        {editing && (
          <button type="button" onClick={onCancelEdit} className={secondaryButton}>
            Cancelar edición
          </button>
        )}
      </div>
    </form>
  );
}
