import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, Loader2 } from "lucide-react";
import type { FutureExpenseItem } from "../../api/futureExpenses";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { formatMoney, todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { useFutureBackend, type PaidResult } from "./backend";
import { describeFutureError } from "./errors";
import { defaultPayDraft, toPayInput, validatePay, type AmountErrors, type PayCategory, type PayDraft } from "./form";
import { useInvalidateFuture } from "./useInvalidate";

interface Props {
  item: FutureExpenseItem;
  /** Gasto categories to choose from. */
  categories: PayCategory[];
  /** Her ledger needs the category chosen (no inference); the owner's can be left automatic. */
  categoryRequired?: boolean;
  onClose: () => void;
  onPaid: (result: PaidResult) => void;
}

/**
 * Confirmation of "Marcar pagado". The amount starts at the target and is what was really paid. Paying registers
 * the Gasto, releases what was saved for the item (what was not used goes back to the free balance) and closes it.
 */
export function PayDialog({ item, categories, categoryRequired = false, onClose, onPaid }: Props) {
  const backend = useFutureBackend();
  const invalidate = useInvalidateFuture();
  const [draft, setDraft] = useState<PayDraft>(() => defaultPayDraft(item, todayISO()));
  const [errors, setErrors] = useState<AmountErrors>({});
  const amountRef = useRef<HTMLInputElement>(null);
  const categoryRef = useRef<HTMLSelectElement>(null);
  const set = (patch: Partial<PayDraft>) => setDraft((d) => ({ ...d, ...patch }));

  const pay = useMutation({
    mutationFn: () => backend.pay(item.id, toPayInput(draft)),
    onSuccess: async (result) => {
      await invalidate();
      onPaid(result);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    pay.reset();
    const found = validatePay(draft, categoryRequired);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      else if (found.category) categoryRef.current?.focus();
      return;
    }
    pay.mutate();
  }

  return (
    <Modal title={`Marcar pagado ${item.name}`} onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm text-muted">
          Se registrará un gasto con el monto que pagaste y se liberará el ahorro de este gasto ({formatMoney(item.saved)}). Si ahorraste más de lo que
          pagaste, la diferencia vuelve al saldo libre. Esta acción no se puede repetir.
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            label="Monto pagado (MXN)"
            inputMode="decimal"
            inputRef={amountRef}
            value={draft.amount}
            onChange={(e) => set({ amount: e.target.value })}
            error={errors.amount}
            hint={`Meta: ${formatMoney(item.target_amount)}.`}
            autoComplete="off"
          />
          <TextField label="Fecha de pago" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
          <SelectField
            label="Categoría del gasto"
            selectRef={categoryRef}
            value={draft.category}
            onChange={(e) => set({ category: e.target.value })}
            error={errors.category}
            hint={categoryRequired ? "Elige una de tus categorías de gasto." : "Automática la deduce del nombre."}
          >
            <option value="">{categoryRequired ? "Elige una categoría" : "Automática"}</option>
            {categories.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </SelectField>
        </div>
        {pay.isError && <ErrorBanner>{describeFutureError(pay.error)}</ErrorBanner>}
        <div className="flex flex-wrap gap-3">
          <button type="submit" disabled={pay.isPending} aria-busy={pay.isPending} className={primaryButton}>
            {pay.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <CheckCircle2 className="h-4 w-4" aria-hidden="true" />}
            {pay.isPending ? "Guardando…" : "Confirmar pago"}
          </button>
          <button type="button" onClick={onClose} disabled={pay.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
