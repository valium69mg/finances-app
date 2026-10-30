import { useQuery } from "@tanstack/react-query";
import { expenseRequestsKeys, listExpenseRequests } from "../api/expenseRequests";

/** Count of requests waiting for the owner, shown next to the Peticiones tab. A failure just hides it. */
export function PendingRequestsBadge({ className = "ml-auto" }: { className?: string }) {
  const pending = useQuery({
    queryKey: expenseRequestsKeys.pendingCount,
    queryFn: async () => (await listExpenseRequests("solicitada")).length,
    retry: false,
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  if (!pending.data) return null;
  return (
    <span
      data-testid="pending-requests-badge"
      aria-label={pending.data === 1 ? "1 petición pendiente" : `${pending.data} peticiones pendientes`}
      className={`${className} inline-flex h-6 min-w-6 shrink-0 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-semibold text-on-primary`}
    >
      {pending.data}
    </span>
  );
}
