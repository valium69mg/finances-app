import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import { useFocusOnEdit } from "../../hooks/useFocusOnEdit";
import { createExpense, expensesKeys, inferCategory, updateExpense, type Expense, type ExpenseInput, type SaveExpenseResult } from "../../api/expenses";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { cycleOf, cycleRangeLabel } from "../cycle";
import { ErrorBanner, fieldGrid, primaryButton, secondaryButton } from "../settings/ui";
import { describeExpenseError } from "./errors";
import { DEBOUNCE_MS, useDebounced, withValue } from "./formHelpers";
import { isPositiveDecimal, todayISO } from "./money";
import { SelectField } from "./SelectField";

interface Props {
  settings: AllSettings;
  /** When set, the form edits this expense instead of creating one. Remount (key) to switch. */
  editing: Expense | null;
  onSaved: (result: SaveExpenseResult) => void;
  onCancelEdit: () => void;
}

export function ExpenseForm({ settings, editing, onSaved, onCancelEdit }: Props) {
  const qc = useQueryClient();
  const methods = settings.payment_methods;
  const defaultMethod = methods.includes("Débito") ? "Débito" : (methods[0] ?? "");

  const [description, setDescription] = useState(editing?.description ?? "");
  const [amount, setAmount] = useState(editing?.amount ?? "");
  const [currency, setCurrency] = useState(editing?.currency ?? "MXN");
  const [rate, setRate] = useState(editing?.exchange_rate ?? "");
  const [date, setDate] = useState(editing?.date ?? todayISO());
  const [method, setMethod] = useState(editing?.payment_method ?? defaultMethod);
  // The user's own category choice; null means "follow the suggestion".
  const [chosen, setChosen] = useState<string | null>(editing ? editing.category : null);
  const [errors, setErrors] = useState<{ amount?: string; rate?: string; date?: string }>({});
  const formRef = useFocusOnEdit<HTMLFormElement>(editing !== null);
  const amountRef = useRef<HTMLInputElement>(null);

  const debouncedDescription = useDebounced(description.trim(), DEBOUNCE_MS);
  const inferQuery = useQuery({
    queryKey: expensesKeys.infer(debouncedDescription),
    queryFn: () => inferCategory(debouncedDescription),
    enabled: debouncedDescription.length >= 2 && chosen === null,
    retry: false,
    staleTime: 60_000,
  });
  const suggestion = chosen === null && debouncedDescription.length >= 2 ? (inferQuery.data ?? null) : null;
  const category = chosen ?? suggestion ?? "";

  const save = useMutation({
    mutationFn: (input: ExpenseInput) => (editing ? updateExpense(editing.id, input) : createExpense(input)),
    onSuccess: (result) => {
      void qc.invalidateQueries({ queryKey: expensesKeys.all });
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
    if (!isPositiveDecimal(amount)) found.amount = "Escribe un monto mayor a cero, por ejemplo 250.50.";
    if (currency === "USD" && rate.trim() !== "" && !isPositiveDecimal(rate)) found.rate = "Escribe un tipo de cambio válido, por ejemplo 17.50.";
    if (!date) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    const input: ExpenseInput = { amount: amount.trim(), currency, date, payment_method: method };
    if (description.trim()) input.description = description.trim();
    if (category) input.category = category;
    if (currency === "USD" && rate.trim()) input.exchange_rate = rate.trim();
    save.mutate(input);
  }

  const categoryOptions = withValue(
    settings.categories.map((c) => c.name),
    category,
  );
  const methodOptions = withValue(methods, method);
  const cycleStartDay = settings.general.cycle_start_day;
  const periodHint = date ? `Se cuenta en el periodo ${cycleRangeLabel(cycleOf(date, cycleStartDay), cycleStartDay)}` : "";
  const title = editing ? "Editar gasto" : "Nuevo gasto";

  return (
    <form ref={formRef} noValidate onSubmit={onSubmit} aria-labelledby="expense-form-title" className="space-y-5">
      <h2 id="expense-form-title" className="text-lg font-semibold tracking-tight">
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
        <TextField
          label="Fecha"
          type="date"
          hint={periodHint || undefined}
          value={date}
          onChange={(e) => setDate(e.target.value)}
          error={errors.date}
        />
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

      {save.isError && <ErrorBanner>{describeExpenseError(save.error)}</ErrorBanner>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : editing ? (
            <Save className="h-4 w-4" aria-hidden="true" />
          ) : (
            <Plus className="h-4 w-4" aria-hidden="true" />
          )}
          {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Agregar gasto"}
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
