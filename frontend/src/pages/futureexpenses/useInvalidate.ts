import { useQueryClient } from "@tanstack/react-query";
import { dashboardKeys } from "../../api/dashboard";
import { expensesKeys } from "../../api/expenses";
import { futureExpensesKeys } from "../../api/futureExpenses";
import { savingsKeys } from "../../api/savings";

/**
 * Refreshes everything a future expense change can touch: the items, the dashboard card, the savings
 * (contributions, assignments and releases are Ahorro movements) and the expenses (a payment registers one).
 */
export function useInvalidateFuture() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: futureExpensesKeys.all }),
      qc.invalidateQueries({ queryKey: dashboardKeys.all }),
      qc.invalidateQueries({ queryKey: savingsKeys.all }),
      qc.invalidateQueries({ queryKey: expensesKeys.all }),
    ]);
}
