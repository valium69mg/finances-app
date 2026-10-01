import { useId, useRef, useState, type FormEvent } from "react";
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { CheckCircle2, Loader2 } from "lucide-react";
import { approveExpenseRequest, expenseRequestsKeys, getBudgetCheck, type ApproveResult, type ExpenseRequest, type RequestDestination } from "../../api/expenseRequests";
import { TextField } from "../../components/AuthCard";
import { BudgetFitBar } from "../../components/BudgetFitBar";
import { Modal } from "../../components/Modal";
import { formatMoney } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { dateLabel } from "../taxfiling/labels";
import { describeRequestError } from "./errors";
import { toApproveInput, validateApproval, type ApproveDraft, type ApproveErrors } from "./form";
import { useInvalidateRequests } from "./useInvalidate";

interface Props {
  request: ExpenseRequest;
  /** Gasto category names to choose from. */
  categories: string[];
  /** Payment methods of the settings; the default is Débito when it exists. */
  paymentMethods: string[];
  onClose: () => void;
  onApproved: (result: ApproveResult) => void;
}

const isDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v);

const OPTIONS: { value: RequestDestination; title: string; text: string }[] = [
  { value: "gasto", title: "Registrar como gasto", text: "Se crea el gasto real y cuenta contra el presupuesto de su categoría." },
  { value: "gasto_futuro", title: "Mover a gasto futuro", text: "Se agrega a Gastos futuros para juntarlo con calma." },
];

/**
 * Approval: the owner picks the destination. As a Gasto, the dialog shows the budget check live (it refetches
 * when the category or the date change) and a request that does not fit can still be approved, with the excess
 * written out. As a future expense there is no budget check: it does not touch this cycle.
 */
export function ApproveDialog({ request, categories, paymentMethods, onClose, onApproved }: Props) {
  const invalidate = useInvalidateRequests();
  const groupId = useId();
  const [draft, setDraft] = useState<ApproveDraft>(() => ({
    destination: "gasto",
    category: request.suggested_category && categories.includes(request.suggested_category) ? request.suggested_category : (categories[0] ?? ""),
    date: request.expense_date,
    paymentMethod: paymentMethods.includes("Débito") ? "Débito" : (paymentMethods[0] ?? ""),
    dueDate: "",
  }));
  const [errors, setErrors] = useState<ApproveErrors>({});
  const dueRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<ApproveDraft>) => setDraft((d) => ({ ...d, ...patch }));
  const asExpense = draft.destination === "gasto";

  // Live budget check: a new category or date is a new key, so it refetches by itself.
  const checkDate = isDate(draft.date) ? draft.date : "";
  const check = useQuery({
    queryKey: expenseRequestsKeys.budgetCheck(request.id, draft.category, checkDate),
    queryFn: () => getBudgetCheck(request.id, draft.category, checkDate),
    enabled: asExpense && draft.category !== "" && checkDate !== "",
    placeholderData: keepPreviousData,
    retry: false,
  });

  const approve = useMutation({
    mutationFn: () => approveExpenseRequest(request.id, toApproveInput(draft)),
    onSuccess: async (result) => {
      await invalidate();
      onApproved(result);
    },
    // A request decided meanwhile (409) or gone: refresh so the list shows the real state.
    onError: () => void invalidate(),
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    approve.reset();
    const found = validateApproval(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.dueDate) dueRef.current?.focus();
      return;
    }
    approve.mutate();
  }

  return (
    <Modal title="Aprobar petición" onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm">
          <span className="font-medium">{request.description}</span> · <span className="tabular-nums">{formatMoney(request.amount)}</span>
          <span className="text-muted">
            {" "}
            · {dateLabel(request.expense_date)} · pidió {request.requester_email}
          </span>
        </p>

        <fieldset className="space-y-2">
          <legend className="mb-1.5 text-sm font-medium">¿Qué hacemos con esta petición?</legend>
          {OPTIONS.map((o) => {
            const id = `${groupId}-${o.value}`;
            const selected = draft.destination === o.value;
            return (
              <label
                key={o.value}
                htmlFor={id}
                className={`flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors duration-200 focus-within:ring-2 focus-within:ring-ring ${
                  selected ? "border-primary bg-primary/5" : "border-border hover:bg-primary/5"
                }`}
              >
                <input
                  id={id}
                  type="radio"
                  name={`${groupId}-destination`}
                  value={o.value}
                  checked={selected}
                  onChange={() => {
                    setErrors({});
                    set({ destination: o.value });
                  }}
                  className="mt-1 h-5 w-5 shrink-0 accent-primary"
                />
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{o.title}</span>
                  <span className="block text-sm text-muted">{o.text}</span>
                </span>
              </label>
            );
          })}
        </fieldset>

        {asExpense ? (
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <SelectField label="Categoría del gasto" value={draft.category} onChange={(e) => set({ category: e.target.value })} aria-invalid={errors.category ? true : undefined}>
                {categories.length === 0 && <option value="">Sin categorías</option>}
                {categories.map((c) => (
                  <option key={c} value={c}>
                    {c}
                    {c === request.suggested_category ? " (sugerida)" : ""}
                  </option>
                ))}
              </SelectField>
              <TextField label="Fecha del gasto" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
              {paymentMethods.length > 0 && (
                <SelectField label="Método de pago" value={draft.paymentMethod} onChange={(e) => set({ paymentMethod: e.target.value })}>
                  {paymentMethods.map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </SelectField>
              )}
            </div>
            {errors.category && <ErrorBanner>{errors.category}</ErrorBanner>}

            <section aria-label="Revisión del presupuesto" aria-busy={check.isFetching} className="space-y-2">
              <h3 className="text-sm font-semibold">Presupuesto de {draft.category || "la categoría"}</h3>
              {check.data && <BudgetFitBar check={check.data} />}
              {check.isPending && check.fetchStatus !== "idle" && (
                <p role="status" className="flex items-center gap-2 text-sm text-muted">
                  <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                  Revisando el presupuesto…
                </p>
              )}
              {check.isError && <ErrorBanner>{`No se pudo revisar el presupuesto. ${describeRequestError(check.error)} Aun así puedes aprobar la petición.`}</ErrorBanner>}
            </section>
          </div>
        ) : (
          <div className="space-y-3">
            <TextField label="Fecha de vencimiento" type="date" inputRef={dueRef} value={draft.dueDate} onChange={(e) => set({ dueDate: e.target.value })} error={errors.dueDate} />
            <p className="text-sm text-muted">No cuenta contra el presupuesto de este ciclo: no se revisa el presupuesto.</p>
          </div>
        )}

        {approve.isError && <ErrorBanner>{describeRequestError(approve.error)}</ErrorBanner>}
        <div className="flex flex-wrap gap-3">
          <button type="submit" disabled={approve.isPending} aria-busy={approve.isPending} className={primaryButton}>
            {approve.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <CheckCircle2 className="h-4 w-4" aria-hidden="true" />}
            {approve.isPending ? "Aprobando…" : "Aprobar"}
          </button>
          <button type="button" onClick={onClose} disabled={approve.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
