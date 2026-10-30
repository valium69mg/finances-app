import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { createExpense, expensesKeys, type ExpenseInput, type SaveExpenseResult } from "../api/expenses";
import { dashboardKeys } from "../api/dashboard";
import { listPersonalCategories, personalKeys } from "../api/personal";
import { getSettings, settingsKeys, type AllSettings } from "../api/settings";
import { TextField } from "./AuthCard";
import { Modal } from "./Modal";
import { describeExpenseError } from "../pages/expenses/errors";
import { withValue } from "../pages/expenses/formHelpers";
import { isPositiveDecimal, todayISO } from "../pages/expenses/money";
import { SelectField } from "../pages/expenses/SelectField";
import { MAX_MOVEMENT_DESCRIPTION_LENGTH } from "../pages/movementErrors";
import { PERSONAL_PAGES } from "../pages/personal/config";
import { describePersonalError } from "../pages/personal/errors";
import { MovementDialog } from "../pages/personal/MovementDialog";
import { ErrorBanner, primaryButton, secondaryButton } from "../pages/settings/ui";

/** What the dock shows after a quick expense is saved: the owner gets the budget feedback of the save. */
export type QuickExpenseResult = SaveExpenseResult | null;

interface Props {
  role: "owner" | "household";
  onClose: () => void;
  onSaved: (result: QuickExpenseResult) => void;
}

/**
 * Quick expense behind the dock's "+" button. The owner's reduced form (monto, categoría, descripción, fecha, método de
 * pago) posts to the same endpoint as the Gastos page; the household role reuses her own movement dialog (kind Gasto).
 */
export function QuickExpense({ role, onClose, onSaved }: Props) {
  return role === "owner" ? <OwnerQuickExpense onClose={onClose} onSaved={onSaved} /> : <HouseholdQuickExpense onClose={onClose} onSaved={onSaved} />;
}

function Loading({ onClose, title }: { onClose: () => void; title: string }) {
  return (
    <Modal title={title} onClose={onClose}>
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando…
      </p>
    </Modal>
  );
}

function OwnerQuickExpense({ onClose, onSaved }: Omit<Props, "role">) {
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  if (settings.isPending) return <Loading title="Gasto rápido" onClose={onClose} />;
  if (settings.isError)
    return (
      <Modal title="Gasto rápido" onClose={onClose}>
        <div className="space-y-3">
          <ErrorBanner>{describeExpenseError(settings.error)}</ErrorBanner>
          <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
            Reintentar
          </button>
        </div>
      </Modal>
    );
  return <OwnerForm settings={settings.data} onClose={onClose} onSaved={onSaved} />;
}

function OwnerForm({ settings, onClose, onSaved }: { settings: AllSettings; onClose: () => void; onSaved: (r: QuickExpenseResult) => void }) {
  const qc = useQueryClient();
  const methods = settings.payment_methods;
  const [amount, setAmount] = useState("");
  const [category, setCategory] = useState("");
  const [description, setDescription] = useState("");
  const [date, setDate] = useState(todayISO());
  const [method, setMethod] = useState(methods.includes("Débito") ? "Débito" : (methods[0] ?? ""));
  const [errors, setErrors] = useState<{ amount?: string; date?: string }>({});
  const amountRef = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: (input: ExpenseInput) => createExpense(input),
    onSuccess: async (result) => {
      await Promise.all([qc.invalidateQueries({ queryKey: expensesKeys.all }), qc.invalidateQueries({ queryKey: dashboardKeys.all })]);
      onSaved(result);
      onClose();
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: typeof errors = {};
    if (!isPositiveDecimal(amount)) found.amount = "Escribe un monto mayor a cero, por ejemplo 250.50.";
    if (!date) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    const input: ExpenseInput = { amount: amount.trim(), currency: "MXN", date, payment_method: method };
    if (description.trim()) input.description = description.trim();
    if (category) input.category = category;
    save.mutate(input);
  }

  return (
    <Modal title="Gasto rápido" onClose={onClose}>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <div className="grid gap-4">
          <TextField label="Monto (MXN)" inputMode="decimal" inputRef={amountRef} value={amount} onChange={(e) => setAmount(e.target.value)} error={errors.amount} autoComplete="off" />
          <SelectField label="Categoría" value={category} onChange={(e) => setCategory(e.target.value)}>
            <option value="">Automática (según la descripción)</option>
            {withValue(
              settings.categories.map((c) => c.name),
              category,
            ).map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </SelectField>
          <TextField label="Descripción" value={description} onChange={(e) => setDescription(e.target.value)} maxLength={MAX_MOVEMENT_DESCRIPTION_LENGTH} autoComplete="off" />
          <TextField label="Fecha" type="date" value={date} onChange={(e) => setDate(e.target.value)} error={errors.date} />
          <SelectField label="Método de pago" value={method} onChange={(e) => setMethod(e.target.value)}>
            {withValue(methods, method).map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </SelectField>
        </div>
        {save.isError && <ErrorBanner>{describeExpenseError(save.error)}</ErrorBanner>}
        <div className="flex flex-wrap items-center gap-3">
          <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
            {save.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Plus className="h-4 w-4" aria-hidden="true" />}
            {save.isPending ? "Guardando…" : "Agregar gasto"}
          </button>
          <button type="button" onClick={onClose} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}

function HouseholdQuickExpense({ onClose, onSaved }: Omit<Props, "role">) {
  const config = PERSONAL_PAGES.Gasto;
  const categories = useQuery({ queryKey: personalKeys.categories("Gasto"), queryFn: () => listPersonalCategories("Gasto"), retry: false });
  if (categories.isPending) return <Loading title={config.newLabel} onClose={onClose} />;
  if (categories.isError)
    return (
      <Modal title={config.newLabel} onClose={onClose}>
        <div className="space-y-3">
          <ErrorBanner>{describePersonalError(categories.error)}</ErrorBanner>
          <button type="button" onClick={() => void categories.refetch()} className={secondaryButton}>
            Reintentar
          </button>
        </div>
      </Modal>
    );
  return <MovementDialog config={config} categories={categories.data} editing={null} onClose={onClose} onSaved={() => onSaved(null)} />;
}
