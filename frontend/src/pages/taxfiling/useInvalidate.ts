import { useQueryClient } from "@tanstack/react-query";
import { dashboardKeys } from "../../api/dashboard";
import { expensesKeys } from "../../api/expenses";
import { invoiceKeys } from "../../api/invoices";
import { taxFilingKeys } from "../../api/taxFiling";

/**
 * Refreshes everything a filing operation touches: the filings, the invoices it
 * links (declaration_period), the expense it may create and the dashboard card.
 */
export function useInvalidateAfterFiling() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: taxFilingKeys.all }),
      qc.invalidateQueries({ queryKey: invoiceKeys.all }),
      qc.invalidateQueries({ queryKey: expensesKeys.all }),
      qc.invalidateQueries({ queryKey: dashboardKeys.all }),
    ]);
}
