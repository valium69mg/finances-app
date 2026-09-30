import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Wallet } from "lucide-react";
import { payBill, type Bill, type PayResult } from "../../api/bills";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { withValue } from "../expenses/formHelpers";
import { formatMoney, todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { dateLabel } from "../taxfiling/labels";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { describeBillsError } from "./errors";
import { defaultPayDraft, toPayInput, validatePay, type PayErrors } from "./form";
import { useInvalidateAfterPayment } from "./useInvalidate";

interface Props {
  bill: Bill;
  /** Expense category names to choose from. */
  categories: string[];
  onClose: () => void;
  onPaid: (result: PayResult) => void;
}

/**
 * Registers the payment of a bill's pending occurrence as an expense. The
 * amount starts at the bill's fixed amount (empty for a variable bill, where it
 * is required) and can always be changed; the category starts at the bill's.
 */
export function PayDialog({ bill, categories, onClose, onPaid }: Props) {
  const invalidate = useInvalidateAfterPayment();
  const [draft, setDraft] = useState(() => defaultPayDraft(bill, todayISO()));
  const [errors, setErrors] = useState<PayErrors>({});
  const amountRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<typeof draft>) => setDraft((d) => ({ ...d, ...patch }));

  const pay = useMutation({
    mutationFn: () => payBill(bill.id, toPayInput(draft)),
    onSuccess: async (result) => {
      await invalidate();
      onPaid(result);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    pay.reset();
    const found = validatePay(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    pay.mutate();
  }

  const due = bill.pending_occurrence?.due_date ?? bill.next_due_date;
  const options = withValue(categories, draft.category);

  return (
    <Modal title={`Pagar ${bill.name}`} onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm text-muted">
          Vencimiento del {dateLabel(due)}.{" "}
          {bill.amount === null
            ? "Este pago no tiene monto fijo: escribe cuánto pagaste."
            : `Monto habitual: ${formatMoney(bill.amount)} ${bill.currency}. Cámbialo si pagaste otra cantidad.`}{" "}
          Se registra un gasto y se genera el siguiente vencimiento.
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            label={`Monto (${bill.currency})`}
            inputMode="decimal"
            inputRef={amountRef}
            value={draft.amount}
            onChange={(e) => set({ amount: e.target.value })}
            error={errors.amount}
            autoComplete="off"
          />
          <TextField label="Fecha de pago" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
          <SelectField label="Categoría" value={draft.category} onChange={(e) => set({ category: e.target.value })}>
            {options.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </SelectField>
          <TextField
            label="Descripción (opcional)"
            hint={`Si la dejas vacía se usa “${bill.name}”.`}
            value={draft.description}
            onChange={(e) => set({ description: e.target.value })}
            autoComplete="off"
          />
        </div>
        {pay.isError && <ErrorBanner>{describeBillsError(pay.error)}</ErrorBanner>}
        <div className="flex flex-wrap gap-3">
          <button type="submit" disabled={pay.isPending} aria-busy={pay.isPending} className={primaryButton}>
            {pay.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Wallet className="h-4 w-4" aria-hidden="true" />}
            {pay.isPending ? "Guardando…" : "Registrar pago"}
          </button>
          <button type="button" onClick={onClose} disabled={pay.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
