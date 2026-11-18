import { Modal } from "./Modal";
import { DeleteAction } from "./DeleteAction";
import { ErrorBanner } from "../pages/settings/ui";
import { formatMoney } from "../pages/expenses/money";

interface Props {
  /** Description of the assignment ("Asignado a Viaje"). */
  label: string;
  date: string;
  amount: string;
  currency?: string;
  pending: boolean;
  /** Message of the failed deletion, if any. */
  error?: string | null;
  onStart?: () => void;
  onConfirm: () => void;
  onClose: () => void;
}

/**
 * A future expense assignment is two linked rows (out of the free balance, into the item), shown as one. It has no
 * edit form: the dialog shows it and deletes both rows after a confirmation, which returns the money to the saldo libre.
 */
export function AssignmentDialog({ label, date, amount, currency = "MXN", pending, error, onStart, onConfirm, onClose }: Props) {
  return (
    <Modal title="Asignación" onClose={onClose}>
      <div className="space-y-5">
        <p className="break-words font-medium">{label}</p>
        <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm tabular-nums">
          <dt className="text-muted">Fecha</dt>
          <dd className="text-right">{date}</dd>
          <dt className="text-muted">Monto</dt>
          <dd className="text-right">
            {formatMoney(amount.replace(/^-/, ""))} {currency}
          </dd>
        </dl>
        <p className="text-sm text-muted">Las asignaciones no se editan. Si la eliminas, el dinero regresa al saldo libre.</p>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        <DeleteAction
          name={label}
          label="Eliminar asignación"
          question="¿Eliminar esta asignación? El dinero regresará al saldo libre."
          pending={pending}
          onStart={onStart}
          onConfirm={onConfirm}
        />
      </div>
    </Modal>
  );
}
