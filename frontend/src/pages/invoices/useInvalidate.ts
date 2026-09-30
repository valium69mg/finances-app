import { useQueryClient } from "@tanstack/react-query";
import { dashboardKeys } from "../../api/dashboard";
import { invoiceKeys } from "../../api/invoices";
import { taxFilingKeys } from "../../api/taxFiling";

/**
 * Refreshes everything issuing or cancelling an invoice touches: the invoices,
 * the tax filing views that are computed from them (tax preview, filing detail,
 * pending periods, filed records and the invoices left out of a filed period)
 * and the dashboard tax card.
 */
export function useInvalidateAfterInvoiceChange() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: invoiceKeys.all }),
      qc.invalidateQueries({ queryKey: taxFilingKeys.all }),
      qc.invalidateQueries({ queryKey: dashboardKeys.all }),
    ]);
}
