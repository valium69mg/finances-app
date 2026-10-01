import { useQueryClient } from "@tanstack/react-query";
import { dashboardKeys } from "../../api/dashboard";
import { expenseRequestsKeys } from "../../api/expenseRequests";
import { expensesKeys } from "../../api/expenses";
import { futureExpensesKeys } from "../../api/futureExpenses";

/**
 * Refreshes everything a decision can touch: the requests (and the owner's pending count), plus the dashboard,
 * the expenses and the future expenses, which an approval writes to.
 */
export function useInvalidateRequests() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: expenseRequestsKeys.all }),
      qc.invalidateQueries({ queryKey: dashboardKeys.all }),
      qc.invalidateQueries({ queryKey: expensesKeys.all }),
      qc.invalidateQueries({ queryKey: futureExpensesKeys.all }),
    ]);
}
