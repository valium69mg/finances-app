import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import { useFocusOnEdit } from "../../hooks/useFocusOnEdit";
import { billsKeys, createBill, updateBill, type Bill } from "../../api/bills";
import { TextField } from "../../components/AuthCard";
import { withValue } from "../expenses/formHelpers";
import { todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { Checkbox, ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeBillsError } from "./errors";
import { draftOf, emptyDraft, toBillInput, validateBill, type BillDraft, type BillErrors } from "./form";
import { RECURRENCES, RECURRENCE_LABEL } from "./labels";

interface Props {
  /** Expense category names to choose from. */
  categories: string[];
  /** When set, the form edits this bill instead of creating one. Remount (key) to switch. */
  editing: Bill | null;
  onSaved: (bill: Bill, wasEdit: boolean) => void;
  onCancelEdit: () => void;
}

/** Create / edit form of a recurring bill or subscription. */
export function BillForm({ categories, editing, onSaved, onCancelEdit }: Props) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<BillDraft>(() => (editing ? draftOf(editing) : emptyDraft(todayISO(), categories[0] ?? "")));
  const [errors, setErrors] = useState<BillErrors>({});
  const formRef = useFocusOnEdit<HTMLFormElement>(editing !== null);
  const nameRef = useRef<HTMLInputElement>(null);
  const amountRef = useRef<HTMLInputElement>(null);
  const leadRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<BillDraft>) => setDraft((d) => ({ ...d, ...patch }));

  const save = useMutation({
    mutationFn: () => {
      const input = toBillInput(draft, editing ? editing.active : true);
      return editing ? updateBill(editing.id, input) : createBill(input);
    },
    onSuccess: (bill) => {
      void qc.invalidateQueries({ queryKey: billsKeys.all });
      onSaved(bill, editing !== null);
      if (!editing) setDraft(emptyDraft(todayISO(), draft.category));
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    save.reset();
    const found = validateBill(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.name) nameRef.current?.focus();
      else if (found.amount) amountRef.current?.focus();
      else if (found.lead) leadRef.current?.focus();
      return;
    }
    save.mutate();
  }

  const categoryOptions = withValue(categories, draft.category);
  const title = editing ? `Editar ${editing.name}` : "Nuevo pago recurrente";

  return (
    <form ref={formRef} noValidate onSubmit={onSubmit} aria-labelledby="bill-form-title" className="space-y-5">
      <h2 id="bill-form-title" className="text-lg font-semibold tracking-tight">
        {title}
      </h2>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <TextField label="Nombre" inputRef={nameRef} value={draft.name} onChange={(e) => set({ name: e.target.value })} error={errors.name} autoComplete="off" />
        <SelectField label="Categoría del gasto" value={draft.category} onChange={(e) => set({ category: e.target.value })} hint="Puedes cambiarla al pagar.">
          {draft.category === "" && <option value="">Elige una categoría</option>}
          {categoryOptions.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </SelectField>
        <SelectField label="Recurrencia" value={draft.recurrence} onChange={(e) => set({ recurrence: e.target.value as BillDraft["recurrence"] })}>
          {RECURRENCES.map((r) => (
            <option key={r} value={r}>
              {RECURRENCE_LABEL[r]}
            </option>
          ))}
        </SelectField>
        <TextField
          label="Monto"
          inputMode="decimal"
          inputRef={amountRef}
          value={draft.variable ? "" : draft.amount}
          onChange={(e) => set({ amount: e.target.value })}
          disabled={draft.variable}
          error={errors.amount}
          hint={draft.variable ? "Sin monto fijo: lo escribes cada vez que pagas." : "Al pagar se propone este monto y puedes cambiarlo."}
          autoComplete="off"
        />
        <SelectField label="Moneda" value={draft.currency} onChange={(e) => set({ currency: e.target.value })}>
          <option value="MXN">MXN</option>
          <option value="USD">USD</option>
        </SelectField>
        <TextField
          label="Próximo vencimiento"
          type="date"
          value={draft.nextDueDate}
          onChange={(e) => set({ nextDueDate: e.target.value })}
          error={errors.date}
          hint={editing ? "Cambiar la fecha mueve el vencimiento pendiente; la recurrencia y el monto aplican desde el siguiente." : undefined}
        />
        <TextField
          label="Avisar con (días de anticipación)"
          inputMode="numeric"
          inputRef={leadRef}
          value={draft.leadDays}
          onChange={(e) => set({ leadDays: e.target.value })}
          error={errors.lead}
          hint="Marca “Vence pronto” dentro de ese plazo. Los correos de aviso llegarán más adelante."
          autoComplete="off"
        />
        <TextField label="Notas (opcional)" value={draft.notes} onChange={(e) => set({ notes: e.target.value })} autoComplete="off" />
        <div className="self-end">
          <Checkbox label="Monto variable (sin monto fijo)" checked={draft.variable} onChange={(variable) => set({ variable })} />
        </div>
      </div>

      {save.isError && <ErrorBanner>{describeBillsError(save.error)}</ErrorBanner>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : editing ? (
            <Save className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Plus className="h-4 w-4" aria-hidden="true" />
          )}
          {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Agregar pago recurrente"}
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
