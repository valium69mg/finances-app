import { useMutation } from "@tanstack/react-query";
import { revertExpenseRequest, type ExpenseRequest } from "../../api/expenseRequests";
import { ApiError } from "../../api/client";
import { formatMoney } from "../expenses/money";
import { ConfirmDialog } from "../monthclose/ConfirmDialog";
import { describeRequestError } from "./errors";
import { useInvalidateRequests } from "./useInvalidate";

interface Props {
  request: ExpenseRequest;
  onClose: () => void;
  onReverted: (request: ExpenseRequest) => void;
}

/** What reverting deletes, in words, so the owner confirms exactly what will be undone. */
export function revertEffect(r: ExpenseRequest): string {
  return r.result_kind === "gasto_futuro"
    ? `Se eliminará el gasto futuro «${r.description}»; su ahorro asignado vuelve al saldo libre.`
    : `Se eliminará el gasto registrado de ${formatMoney(r.amount)}.`;
}

function describeRevertError(err: unknown): string {
  if (err instanceof ApiError && err.code === "invalid_state") {
    return "Esta petición ya no está aprobada (quizá ya la revertiste). Actualiza la lista para ver su estado.";
  }
  return describeRequestError(err);
}

/** Confirmation to send an approved request back to solicitada, undoing what the approval created. */
export function RevertDialog({ request, onClose, onReverted }: Props) {
  const invalidate = useInvalidateRequests();
  const revert = useMutation({
    mutationFn: () => revertExpenseRequest(request.id),
    onSuccess: async (r) => {
      await invalidate();
      onReverted(r);
    },
    // A request changed meanwhile (409) or gone: refresh the list behind the dialog, which stays open with the message.
    onError: () => void invalidate(),
  });

  return (
    <ConfirmDialog
      title="Volver a solicitada"
      confirmLabel="Volver a solicitada"
      cancelLabel="Cancelar"
      destructive
      pending={revert.isPending}
      error={revert.isError ? describeRevertError(revert.error) : null}
      onConfirm={() => revert.mutate()}
      onCancel={onClose}
    >
      <p className="break-words">
        {request.description} · {formatMoney(request.amount)}
      </p>
      <p className="break-words font-medium">{revertEffect(request)}</p>
      <p className="text-muted">La petición volverá a la lista de solicitadas y quien la pidió recibirá un aviso por correo.</p>
    </ConfirmDialog>
  );
}
