import { useQueryClient } from "@tanstack/react-query";
import { useFutureBackend } from "./backend";

/**
 * Refreshes everything a future expense change can touch in the ledger the page works on (see FutureBackend.invalidate):
 * the items, the savings (contributions, assignments and releases are Ahorro movements) and the expenses (a payment
 * registers one), plus the dashboard card for the owner.
 */
export function useInvalidateFuture() {
  const qc = useQueryClient();
  const backend = useFutureBackend();
  return () => backend.invalidate(qc);
}
