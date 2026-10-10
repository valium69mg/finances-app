import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import { useFocusOnEdit } from "../../hooks/useFocusOnEdit";
import { createSaving, savingsKeys, updateSaving, type Saving, type SavingInput } from "../../api/savings";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { withValue } from "../expenses/formHelpers";
import { isNonZeroDecimal, isPositiveDecimal, todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { MAX_MOVEMENT_DESCRIPTION_LENGTH } from "../movementErrors";
import { ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";
import { InstrumentSelect } from "./InstrumentSelect";

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
  const methods = settings.payment_methods;
  const defaultMethod = methods.includes("Transferencia") ? "Transferencia" : (methods[0] ?? "");

  const [description, setDescription] = useState(editing?.description ?? "");
  const [amount, setAmount] = useState(editing?.amount ?? "");
  const [currency, setCurrency] = useState(editing?.currency ?? "MXN");
  const [rate, setRate] = useState(editing?.exchange_rate ?? "");
  // null = the user never touched the date, so it is "today" whenever it is read.
  const [date, setDate] = useState<string | null>(editing?.date ?? null);
  const [method, setMethod] = useState(editing?.payment_method ?? defaultMethod);
  const [instrument, setInstrument] = useState(editing?.instrument ?? "");
  const [category, setCategory] = useState(editing?.category ?? "");
  const [errors, setErrors] = useState<{ amount?: string; rate?: string; date?: string }>({});
  const formRef = useFocusOnEdit<HTMLFormElement>(editing !== null);
  const amountRef = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: (input: SavingInput) => (editing ? updateSaving(editing.id, input) : createSaving(input)),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: savingsKeys.all });
      onSaved();
      if (!editing) {
        setDescription("");
        setAmount("");
        setRate("");
      }
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    // Untouched date: resolve "today" now, not when the form was mounted.
    const effectiveDate = date ?? todayISO();
    const found: typeof errors = {};
    if (!isNonZeroDecimal(amount)) found.amount = "Escribe un monto distinto de cero, por ejemplo 1500.00 o -500.00 para un retiro.";
    if (currency === "USD" && rate.trim() !== "" && !isPositiveDecimal(rate)) found.rate = "Escribe un tipo de cambio válido, por ejemplo 17.50.";
    if (!effectiveDate) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    // Currency, rate and payment method are always sent so an edit can never reset them.
    const input: SavingInput = { amount: amount.trim(), date: effectiveDate, currency, payment_method: method };
    if (description.trim()) input.description = description.trim();
    if (category) input.category = category;
    if (instrument) input.instrument = instrument;
    if (currency === "USD" && rate.trim()) input.exchange_rate = rate.trim();
    save.mutate(input);
  }

  const methodOptions = withValue(methods, method);
  const categoryOptions = withValue(
    settings.categories.filter((c) => c.kind === SAVINGS_KIND).map((c) => c.name),
    category,
  );
  const title = editing ? "Editar ahorro" : "Nuevo ahorro";

  return (
    <form ref={formRef} noValidate onSubmit={onSubmit} aria-labelledby="saving-form-title" className="space-y-5">
      <h2 id="saving-form-title" className="text-lg font-semibold tracking-tight">
        {title}
      </h2>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <InstrumentSelect
          label="Instrumento"
          value={instrument}
          onChange={setInstrument}
          instruments={instruments}
          emptyLabel="Automático (según la categoría)"
        />
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
        <SelectField label="Moneda" value={currency} onChange={(e) => setCurrency(e.target.value)}>
          <option value="MXN">MXN</option>
          <option value="USD">USD</option>
        </SelectField>
        {currency === "USD" && (
          <TextField
            label="Tipo de cambio (opcional)"
            hint={editing?.currency === "USD" ? "Si lo dejas vacío se conserva el actual." : "Si lo dejas vacío se usa el de configuración."}
            inputMode="decimal"
            value={rate}
            onChange={(e) => setRate(e.target.value)}
            error={errors.rate}
            autoComplete="off"
          />
        )}
        <TextField label="Fecha" type="date" value={date ?? todayISO()} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        <SelectField label="Método de pago" value={method} onChange={(e) => setMethod(e.target.value)}>
          {methodOptions.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </SelectField>
        <TextField label="Descripción" value={description} onChange={(e) => setDescription(e.target.value)} maxLength={MAX_MOVEMENT_DESCRIPTION_LENGTH} autoComplete="off" />
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
