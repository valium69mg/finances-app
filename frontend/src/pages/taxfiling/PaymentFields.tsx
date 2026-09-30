import type { Ref } from "react";
import { TextField, FieldError } from "../../components/AuthCard";
import { Checkbox } from "../settings/ui";
import type { PaymentDraft, PaymentErrors } from "./payment";

interface Props {
  draft: PaymentDraft;
  onChange: (draft: PaymentDraft) => void;
  errors: PaymentErrors;
  /** Lets the parent focus the first invalid field. */
  isrRef?: Ref<HTMLInputElement>;
  ivaRef?: Ref<HTMLInputElement>;
}

/**
 * Amounts paid to the SAT, payment date and the optional "record as expense"
 * choice. The expense is never created unless the box is checked.
 */
export function PaymentFields({ draft, onChange, errors, isrRef, ivaRef }: Props) {
  const set = (patch: Partial<PaymentDraft>) => onChange({ ...draft, ...patch });
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-3">
        <TextField label="Fecha de pago" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
        <TextField
          label="ISR pagado (MXN)"
          inputMode="decimal"
          inputRef={isrRef}
          value={draft.isr}
          onChange={(e) => set({ isr: e.target.value })}
          error={errors.isr}
          autoComplete="off"
        />
        <TextField
          label="IVA pagado (MXN)"
          inputMode="decimal"
          inputRef={ivaRef}
          value={draft.iva}
          onChange={(e) => set({ iva: e.target.value })}
          error={errors.iva}
          autoComplete="off"
        />
      </div>
      <div>
        <Checkbox label="Registrar el pago como gasto (Impuestos)" checked={draft.recordExpense} onChange={(recordExpense) => set({ recordExpense })} />
        <p className="text-sm text-muted">Crea un gasto en la categoría Impuestos por el ISR y el IVA pagados, en pesos y con la fecha de pago. Si no lo marcas, no se crea ningún gasto.</p>
        {errors.expense && <FieldError id="payment-expense-error">{errors.expense}</FieldError>}
      </div>
    </div>
  );
}
