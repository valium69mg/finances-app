import { useMutation, useQueryClient } from "@tanstack/react-query";
import { futureExpensesKeys } from "../../api/futureExpenses";
import { deleteSaving, savingsKeys, type Saving } from "../../api/savings";
import type { AllSettings } from "../../api/settings";
import { AssignmentDialog } from "../../components/AssignmentDialog";
import { DeleteAction } from "../../components/DeleteAction";
import { Modal } from "../../components/Modal";
import { formatMoney } from "../expenses/money";
import { ErrorBanner } from "../settings/ui";
import { describeSavingsError } from "./errors";
import { SavingForm } from "./SavingForm";

/** Deletes a saving and refreshes everything the savings feed (list, portfolio, the dashboard through the same keys). */
function useDeleteSaving(onDeleted: () => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteSaving(id),
    onSuccess: async () => {
      onDeleted();
      await qc.invalidateQueries({ queryKey: savingsKeys.all });
      // Deleting an assignment changes the balance of its future expense and the free balance.
      await qc.invalidateQueries({ queryKey: futureExpensesKeys.all });
    },
  });
}

interface SavingDialogProps {
  settings: AllSettings;
  /** The saving to edit, or null for a new one. */
  saving: Saving | null;
  onClose: () => void;
  onSaved: () => void;
  onDeleted: () => void;
}

/** New / edit dialog of a saving; editing also offers "Eliminar" behind a confirmation, so nothing needs a form far above. */
export function SavingDialog({ settings, saving, onClose, onSaved, onDeleted }: SavingDialogProps) {
  const remove = useDeleteSaving(onDeleted);
  return (
    <Modal title={saving ? "Editar ahorro" : "Nuevo ahorro"} onClose={onClose} highlight={saving !== null}>
      <div className="space-y-5">
        <SavingForm key={saving?.id ?? "new"} settings={settings} editing={saving} onSaved={onSaved} onCancel={onClose} />
        {saving && (
          <div className="space-y-3 border-t border-border pt-5">
            {remove.isError && <ErrorBanner>{describeSavingsError(remove.error)}</ErrorBanner>}
            <DeleteAction
              name={saving.description || "este ahorro"}
              question="¿Eliminar este ahorro?"
              pending={remove.isPending}
              onStart={() => remove.reset()}
              onConfirm={() => remove.mutate(saving.id)}
            />
          </div>
        )}
      </div>
    </Modal>
  );
}

/** A transfer leg cannot be edited: the dialog shows it and deletes the whole transfer (both legs) after a confirmation. */
export function TransferLegDialog({ saving, onClose, onDeleted }: { saving: Saving; onClose: () => void; onDeleted: () => void }) {
  const remove = useDeleteSaving(onDeleted);
  const label = saving.description || "Traspaso";
  return (
    <Modal title={label} onClose={onClose}>
      <div className="space-y-5">
        <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm tabular-nums">
          <dt className="text-muted">Fecha</dt>
          <dd className="text-right">{saving.date}</dd>
          <dt className="text-muted">Monto</dt>
          <dd className="text-right">
            {formatMoney(saving.amount)} {saving.currency}
          </dd>
        </dl>
        <p className="text-sm text-muted">Los traspasos no se editan: si te equivocaste, elimínalo y regístralo de nuevo.</p>
        {remove.isError && <ErrorBanner>{describeSavingsError(remove.error)}</ErrorBanner>}
        <DeleteAction
          name={label}
          question="¿Eliminar este traspaso? Se eliminarán ambas partes (salida y entrada)."
          pending={remove.isPending}
          onStart={() => remove.reset()}
          onConfirm={() => remove.mutate(saving.id)}
        />
      </div>
    </Modal>
  );
}

/** A future expense assignment (two linked rows shown as one): deleting it removes both and the money goes back to the saldo libre. */
export function AssignmentLegDialog({ saving, onClose, onDeleted }: { saving: Saving; onClose: () => void; onDeleted: () => void }) {
  const remove = useDeleteSaving(onDeleted);
  return (
    <AssignmentDialog
      label={saving.description || "Asignación"}
      date={saving.date}
      amount={saving.amount}
      currency={saving.currency}
      pending={remove.isPending}
      error={remove.isError ? describeSavingsError(remove.error) : null}
      onStart={() => remove.reset()}
      onConfirm={() => remove.mutate(saving.id)}
      onClose={onClose}
    />
  );
}
