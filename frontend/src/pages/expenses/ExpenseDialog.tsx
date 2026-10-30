import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteExpense, expensesKeys, type Expense, type SaveExpenseResult } from "../../api/expenses";
import type { AllSettings } from "../../api/settings";
import { DeleteAction } from "../../components/DeleteAction";
import { Modal } from "../../components/Modal";
import { ErrorBanner } from "../settings/ui";
import { describeExpenseError } from "./errors";
import { ExpenseForm } from "./ExpenseForm";

interface Props {
  settings: AllSettings;
  expense: Expense;
  onClose: () => void;
  onSaved: (result: SaveExpenseResult) => void;
}

/** Edit dialog of an expense (opened from its row or `?editar=`); "Eliminar" lives here, behind a confirmation. */
export function ExpenseDialog({ settings, expense, onClose, onSaved }: Props) {
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: () => deleteExpense(expense.id),
    onSuccess: async () => {
      onClose();
      await qc.invalidateQueries({ queryKey: expensesKeys.all });
    },
  });
  return (
    <Modal title="Editar gasto" onClose={onClose} highlight>
      <div className="space-y-5">
        <ExpenseForm key={expense.id} settings={settings} editing={expense} onSaved={onSaved} onCancelEdit={onClose} inDialog />
        <div className="space-y-3 border-t border-border pt-5">
          {remove.isError && <ErrorBanner>{describeExpenseError(remove.error)}</ErrorBanner>}
          <DeleteAction
            name={expense.description || "este gasto"}
            question="¿Eliminar este gasto?"
            pending={remove.isPending}
            onStart={() => remove.reset()}
            onConfirm={() => remove.mutate()}
          />
        </div>
      </div>
    </Modal>
  );
}
