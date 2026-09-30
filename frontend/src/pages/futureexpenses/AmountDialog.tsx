import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, PiggyBank } from "lucide-react";
import { addFutureSaving, assignFutureSaving, type FutureExpenseItem } from "../../api/futureExpenses";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { formatMoney, todayISO } from "../expenses/money";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { describeFutureError } from "./errors";
import { isGreater, validateAmount, type AmountDraft, type AmountErrors } from "./form";
import { useInvalidateFuture } from "./useInvalidate";

export type AmountMode = "saving" | "assign";

interface Props {
  item: FutureExpenseItem;
  mode: AmountMode;
  /** Free balance, the most an assignment can take. */
  freeBalance: string;
  onClose: () => void;
  onDone: (message: string) => void;
}

/**
 * One dialog for the two ways money reaches an item: a new saving (an Ahorro in Gastos futuros linked to the
 * item) or an assignment of part of the free balance (a transfer that does not change the savings total).
 */
export function AmountDialog({ item, mode, freeBalance, onClose, onDone }: Props) {
  const invalidate = useInvalidateFuture();
  const [draft, setDraft] = useState<AmountDraft>({ amount: "", date: todayISO() });
  const [errors, setErrors] = useState<AmountErrors>({});
  const amountRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<AmountDraft>) => setDraft((d) => ({ ...d, ...patch }));
  const assign = mode === "assign";

  const run = useMutation({
    mutationFn: async () => {
      const input = { amount: draft.amount.trim(), date: draft.date };
      if (assign) await assignFutureSaving(item.id, input);
      else await addFutureSaving(item.id, input);
    },
    onSuccess: async () => {
      await invalidate();
      onDone(
        assign
          ? `Se asignaron ${formatMoney(draft.amount)} del saldo libre a ${item.name}.`
          : `Se agregaron ${formatMoney(draft.amount)} de ahorro a ${item.name}.`,
      );
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    run.reset();
    const found = validateAmount(draft);
    if (!found.amount && assign && isGreater(draft.amount, freeBalance)) {
      found.amount = `El saldo libre es ${formatMoney(freeBalance)}. Escribe un monto menor o igual.`;
    }
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    run.mutate();
  }

  return (
    <Modal title={assign ? `Asignar saldo libre a ${item.name}` : `Agregar ahorro a ${item.name}`} onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm text-muted">
          {assign
            ? `Saldo libre disponible: ${formatMoney(freeBalance)}. Es ahorro en Gastos futuros que aún no pertenece a ningún gasto; asignarlo no cambia tu ahorro total.`
            : "Se registra un ahorro en la categoría Gastos futuros ligado a este gasto."}
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            label="Monto (MXN)"
            inputMode="decimal"
            inputRef={amountRef}
            value={draft.amount}
            onChange={(e) => set({ amount: e.target.value })}
            error={errors.amount}
            autoComplete="off"
          />
          <TextField label="Fecha" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
        </div>
        {run.isError && <ErrorBanner>{describeFutureError(run.error)}</ErrorBanner>}
        <div className="flex flex-wrap gap-3">
          <button type="submit" disabled={run.isPending} aria-busy={run.isPending} className={primaryButton}>
            {run.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <PiggyBank className="h-4 w-4" aria-hidden="true" />}
            {run.isPending ? "Guardando…" : assign ? "Asignar" : "Agregar ahorro"}
          </button>
          <button type="button" onClick={onClose} disabled={run.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
