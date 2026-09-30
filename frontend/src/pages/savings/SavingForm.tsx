import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import { createSaving, savingsKeys, updateSaving, type Saving, type SavingInput } from "../../api/savings";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { withValue } from "../expenses/formHelpers";
import { isNonZeroDecimal, todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";

interface Props {
  settings: AllSettings;
  /** When set, the form edits this saving instead of creating one. Remount (key) to switch. */
  editing: Saving | null;
  onSaved: () => void;
  onCancelEdit: () => void;
}

export const SAVINGS_KIND = "Ahorro";

export function SavingForm({ settings, editing, onSaved, onCancelEdit }: Props) {
  const qc = useQueryClient();
  const instruments = settings.instruments.instruments;

  const [description, setDescription] = useState(editing?.description ?? "");
  const [amount, setAmount] = useState(editing?.amount ?? "");
  const [date, setDate] = useState(editing?.date ?? todayISO());
  const [instrument, setInstrument] = useState(editing?.instrument ?? "");
  const [category, setCategory] = useState(editing?.category ?? "");
  const [errors, setErrors] = useState<{ amount?: string; date?: string }>({});
  const amountRef = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: (input: SavingInput) => (editing ? updateSaving(editing.id, input) : createSaving(input)),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: savingsKeys.all });
      onSaved();
      if (!editing) {
        setDescription("");
        setAmount("");
      }
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: typeof errors = {};
    if (!isNonZeroDecimal(amount)) found.amount = "Escribe un monto distinto de cero, por ejemplo 1500.00 o -500.00 para un retiro.";
    if (!date) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    const input: SavingInput = { amount: amount.trim(), date };
    if (description.trim()) input.description = description.trim();
    if (category) input.category = category;
    if (instrument) input.instrument = instrument;
    save.mutate(input);
  }

  const categoryOptions = withValue(
    settings.categories.filter((c) => c.kind === SAVINGS_KIND).map((c) => c.name),
    category,
  );
  const title = editing ? "Editar ahorro" : "Nuevo ahorro";

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="saving-form-title" className="space-y-5">
      <h2 id="saving-form-title" className="text-lg font-semibold tracking-tight">
        {title}
      </h2>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <SelectField label="Instrumento" value={instrument} onChange={(e) => setInstrument(e.target.value)}>
          <option value="">Automático (según la categoría)</option>
          {withValue(
            instruments.map((i) => i.id),
            instrument,
          ).map((id) => (
            <option key={id} value={id}>
              {instruments.find((i) => i.id === id)?.name ?? id}
            </option>
          ))}
        </SelectField>
        <SelectField label="Categoría" value={category} onChange={(e) => setCategory(e.target.value)}>
          <option value="">Automática (según la descripción)</option>
          {categoryOptions.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </SelectField>
        <TextField
          label="Monto"
          hint="Usa un monto negativo para registrar un retiro."
          inputMode="decimal"
          inputRef={amountRef}
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          error={errors.amount}
          autoComplete="off"
        />
        <TextField label="Fecha" type="date" value={date} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        <TextField label="Descripción" value={description} onChange={(e) => setDescription(e.target.value)} autoComplete="off" />
      </div>

      {save.isError && <ErrorBanner>{describeSavingsError(save.error)}</ErrorBanner>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : editing ? (
            <Save className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Plus className="h-4 w-4" aria-hidden="true" />
          )}
          {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Agregar ahorro"}
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
