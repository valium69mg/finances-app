import { useQueryClient } from "@tanstack/react-query";
import { billsKeys } from "../../api/bills";
import { dashboardKeys } from "../../api/dashboard";
import { expensesKeys } from "../../api/expenses";

/** Refreshes everything paying a bill touches: the bills, the expense it registers and the dashboard. */
export function useInvalidateAfterPayment() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: billsKeys.all }),
      qc.invalidateQueries({ queryKey: expensesKeys.all }),
      qc.invalidateQueries({ queryKey: dashboardKeys.all }),
    ]);
}
