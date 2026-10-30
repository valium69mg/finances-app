import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteIncome, incomeKeys, type Income, type SaveIncomeResult } from "../../api/income";
import type { AllSettings } from "../../api/settings";
import { DeleteAction } from "../../components/DeleteAction";
import { Modal } from "../../components/Modal";
import { ErrorBanner } from "../settings/ui";
import { describeIncomeError } from "./errors";
import { IncomeForm } from "./IncomeForm";

interface Props {
  settings: AllSettings;
  income: Income;
  onClose: () => void;
  onSaved: (result: SaveIncomeResult) => void;
}

/** Edit dialog of an income (opened from its row or `?editar=`); "Eliminar" lives here, behind a confirmation. */
export function IncomeDialog({ settings, income, onClose, onSaved }: Props) {
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteIncome(income.id),
    onSuccess: async () => {
      onClose();
      await qc.invalidateQueries({ queryKey: incomeKeys.all });
    },
  });
  return (
    <Modal title="Editar ingreso" onClose={onClose} highlight>
      <div className="space-y-5">
        <IncomeForm key={income.id} settings={settings} editing={income} onSaved={onSaved} onCancelEdit={onClose} inDialog />
        <div className="space-y-3 border-t border-border pt-5">
          {remove.isError && <ErrorBanner>{describeIncomeError(remove.error)}</ErrorBanner>}
          <DeleteAction
            name={income.description || "este ingreso"}
            question="¿Eliminar este ingreso?"
            pending={remove.isPending}
            onStart={() => remove.reset()}
            onConfirm={() => remove.mutate()}
          />
        </div>
      </div>
    </Modal>
  );
}
