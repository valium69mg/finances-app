import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import {
  createIncome,
  incomeKeys,
  inferIncomeCategory,
  updateIncome,
  type Income,
  type IncomeInput,
  type SaveIncomeResult,
} from "../../api/income";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { DEBOUNCE_MS, useDebounced, withValue } from "../expenses/formHelpers";
import { isPositiveDecimal, todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeIncomeError } from "./errors";

interface Props {
  settings: AllSettings;
  /** When set, the form edits this income instead of creating one. Remount (key) to switch. */
  editing: Income | null;
  onSaved: (result: SaveIncomeResult) => void;
  onCancelEdit: () => void;
}

export const INCOME_KIND = "Ingreso";

export function IncomeForm({ settings, editing, onSaved, onCancelEdit }: Props) {
  const qc = useQueryClient();
  const methods = settings.payment_methods;
  const defaultMethod = methods.includes("Transferencia") ? "Transferencia" : (methods[0] ?? "");

  const [description, setDescription] = useState(editing?.description ?? "");
  const [amount, setAmount] = useState(editing?.amount ?? "");
  const [currency, setCurrency] = useState(editing?.currency ?? "MXN");
  const [rate, setRate] = useState(editing?.exchange_rate ?? "");
  const [date, setDate] = useState(editing?.date ?? todayISO());
  const [method, setMethod] = useState(editing?.payment_method ?? defaultMethod);
  // The user's own category choice; null means "follow the suggestion".
  const [chosen, setChosen] = useState<string | null>(editing ? editing.category : null);
  const [errors, setErrors] = useState<{ amount?: string; rate?: string; date?: string }>({});
  const amountRef = useRef<HTMLInputElement>(null);

  const debouncedDescription = useDebounced(description.trim(), DEBOUNCE_MS);
  const inferQuery = useQuery({
    queryKey: incomeKeys.infer(debouncedDescription),
    queryFn: () => inferIncomeCategory(debouncedDescription),
    enabled: debouncedDescription.length >= 2 && chosen === null,
    retry: false,
    staleTime: 60_000,
  });
  const suggestion = chosen === null && debouncedDescription.length >= 2 ? (inferQuery.data ?? null) : null;
  const category = chosen ?? suggestion ?? "";

  const save = useMutation({
    mutationFn: (input: IncomeInput) => (editing ? updateIncome(editing.id, input) : createIncome(input)),
    onSuccess: (result) => {
      void qc.invalidateQueries({ queryKey: incomeKeys.all });
      onSaved(result);
      if (!editing) {
        setDescription("");
        setAmount("");
        setRate("");
        setChosen(null);
      }
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: typeof errors = {};
    if (!isPositiveDecimal(amount)) found.amount = "Escribe un monto mayor a cero, por ejemplo 3383.33.";
    if (currency === "USD" && rate.trim() !== "" && !isPositiveDecimal(rate)) found.rate = "Escribe un tipo de cambio válido, por ejemplo 17.50.";
    if (!date) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    const input: IncomeInput = { amount: amount.trim(), currency, date, payment_method: method };
    if (description.trim()) input.description = description.trim();
    if (category) input.category = category;
    if (currency === "USD" && rate.trim()) input.exchange_rate = rate.trim();
    save.mutate(input);
  }

  const categoryOptions = withValue(
    settings.categories.filter((c) => c.kind === INCOME_KIND).map((c) => c.name),
    category,
  );
  const methodOptions = withValue(methods, method);
  const title = editing ? "Editar ingreso" : "Nuevo ingreso";

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="income-form-title" className="space-y-5">
      <h2 id="income-form-title" className="text-lg font-semibold tracking-tight">
        {title}
      </h2>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <TextField label="Descripción" value={description} onChange={(e) => setDescription(e.target.value)} autoComplete="off" />
        <TextField
          label="Monto"
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
            hint="Si lo dejas vacío se usa el de configuración."
            inputMode="decimal"
            value={rate}
            onChange={(e) => setRate(e.target.value)}
            error={errors.rate}
            autoComplete="off"
          />
        )}
        <TextField label="Fecha" type="date" value={date} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        <SelectField label="Método de pago" value={method} onChange={(e) => setMethod(e.target.value)}>
          {methodOptions.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </SelectField>
        <SelectField
          label="Categoría"
          value={category}
          onChange={(e) => setChosen(e.target.value === "" ? null : e.target.value)}
          hint={chosen === null && suggestion ? `Sugerida según la descripción: ${suggestion}. Puedes cambiarla.` : undefined}
        >
          <option value="">Automática (según la descripción)</option>
          {categoryOptions.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </SelectField>
      </div>

      {save.isError && <ErrorBanner>{describeIncomeError(save.error)}</ErrorBanner>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : editing ? (
            <Save className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Plus className="h-4 w-4" aria-hidden="true" />
          )}
          {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Agregar ingreso"}
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
