import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { createExpense, expensesKeys, type ExpenseInput, type SaveExpenseResult } from "../api/expenses";
import { dashboardKeys } from "../api/dashboard";
import { listPersonalCategories, personalKeys } from "../api/personal";
import { getSettings, settingsKeys, type AllSettings } from "../api/settings";
import { keywordOwners } from "../lib/keywords";
import { TextField } from "./AuthCard";
import { DescriptionField } from "./DescriptionField";
import { Modal } from "./Modal";
import { QuickBills } from "./QuickBills";
import { Tabs, panelId, tabId } from "./Tabs";
import { describeExpenseError } from "../pages/expenses/errors";
import { withValue } from "../pages/expenses/formHelpers";
import { expenseCategories } from "../pages/bills/form";
import { isPositiveDecimal, todayISO } from "../pages/expenses/money";
import { SelectField } from "../pages/expenses/SelectField";
import { CATEGORY_NOT_INFERRED_MESSAGE, MAX_MOVEMENT_DESCRIPTION_LENGTH, isCategoryNotInferred } from "../pages/movementErrors";
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
  /** The owner paid a recurring bill from the "Recurrente" tab: the confirmation text to show. */
  onBillPaid: (message: string) => void;
}

/**
 * Quick dialog behind the dock's "+" button. The owner gets two tabs: "Gasto", a reduced form (monto, categoría,
 * descripción, fecha, método de pago) that posts to the same endpoint as the Gastos page, and "Recurrente", which pays a
 * pending recurring bill. The household role reuses her own movement dialog (kind Gasto) and has no tabs.
 */
export function QuickExpense({ role, onClose, onSaved, onBillPaid }: Props) {
  return role === "owner" ? <OwnerQuickExpense onClose={onClose} onSaved={onSaved} onBillPaid={onBillPaid} /> : <HouseholdQuickExpense onClose={onClose} onSaved={onSaved} />;
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

function OwnerQuickExpense({ onClose, onSaved, onBillPaid }: Omit<Props, "role">) {
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
  return <OwnerDialog settings={settings.data} onClose={onClose} onSaved={onSaved} onBillPaid={onBillPaid} />;
}

type OwnerTab = "gasto" | "recurrente";
const OWNER_TABS = [
  { id: "gasto", label: "Gasto" },
  { id: "recurrente", label: "Recurrente" },
] as const;
const TAB_PREFIX = "quick";

/** The owner's dialog: the Gasto form stays mounted (so a draft survives a visit to the other tab) and Recurrente loads when first opened. */
function OwnerDialog({ settings, onClose, onSaved, onBillPaid }: { settings: AllSettings; onClose: () => void; onSaved: (r: QuickExpenseResult) => void; onBillPaid: (message: string) => void }) {
  const [tab, setTab] = useState<OwnerTab>("gasto");
  const [visited, setVisited] = useState(false);
  const categories = expenseCategories(settings);
  function select(id: OwnerTab) {
    setTab(id);
    if (id === "recurrente") setVisited(true);
  }
  return (
    <Modal title={tab === "gasto" ? "Gasto rápido" : "Pago recurrente"} onClose={onClose}>
      <div className="space-y-5">
        <Tabs items={OWNER_TABS} active={tab} onSelect={select} label="Tipo de registro" idPrefix={TAB_PREFIX} />
        <div role="tabpanel" id={panelId(TAB_PREFIX, "gasto")} aria-labelledby={tabId(TAB_PREFIX, "gasto")} hidden={tab !== "gasto"}>
          <OwnerForm settings={settings} onClose={onClose} onSaved={onSaved} />
        </div>
        <div role="tabpanel" id={panelId(TAB_PREFIX, "recurrente")} aria-labelledby={tabId(TAB_PREFIX, "recurrente")} hidden={tab !== "recurrente"}>
          {visited && (
            <QuickBills
              categories={categories}
              onClose={onClose}
              onPaid={(message) => {
                onBillPaid(message);
                onClose();
              }}
            />
          )}
        </div>
      </div>
    </Modal>
  );
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
  const categoryRef = useRef<HTMLSelectElement>(null);

  const save = useMutation({
    mutationFn: (input: ExpenseInput) => createExpense(input),
    // The category could not be deduced: point at the select so the user chooses one.
    onError: (err) => {
      if (isCategoryNotInferred(err)) categoryRef.current?.focus();
    },
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
    <>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <div className="grid gap-4">
          <TextField label="Monto (MXN)" inputMode="decimal" inputRef={amountRef} value={amount} onChange={(e) => setAmount(e.target.value)} error={errors.amount} autoComplete="off" />
          <SelectField
            label="Categoría"
            value={category}
            selectRef={categoryRef}
            error={save.isError && isCategoryNotInferred(save.error) && category === "" ? CATEGORY_NOT_INFERRED_MESSAGE : undefined}
            onChange={(e) => setCategory(e.target.value)}
          >
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
          <DescriptionField
            value={description}
            onChange={setDescription}
            owners={keywordOwners(settings.categories, "Gasto")}
            onPick={(owner) => setCategory(owner.category)}
            maxLength={MAX_MOVEMENT_DESCRIPTION_LENGTH}
          />
          <TextField label="Fecha" type="date" value={date} onChange={(e) => setDate(e.target.value)} error={errors.date} />
          <SelectField label="Método de pago" value={method} onChange={(e) => setMethod(e.target.value)}>
            {withValue(methods, method).map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </SelectField>
        </div>
        {save.isError && !isCategoryNotInferred(save.error) && <ErrorBanner>{describeExpenseError(save.error)}</ErrorBanner>}
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
    </>
  );
}

function HouseholdQuickExpense({ onClose, onSaved }: Omit<Props, "role" | "onBillPaid">) {
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
