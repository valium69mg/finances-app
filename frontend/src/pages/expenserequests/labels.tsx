import { Ban, CheckCircle2, Clock, XCircle, type LucideIcon } from "lucide-react";
import type { ExpenseRequest, RequestStatus } from "../../api/expenseRequests";
import { TONES } from "../../lib/tones";

interface StatusMeta {
  label: string;
  icon: LucideIcon;
  /** Soft background with its strong text color. */
  className: string;
}

/** Icon, text and color of each state: the state is never told by color alone. */
export const STATUS_META: Record<RequestStatus, StatusMeta> = {
  solicitada: { label: "Solicitada", icon: Clock, className: "bg-primary/10 text-primary" },
  aprobada: { label: "Aprobada", icon: CheckCircle2, className: TONES.income.chip },
  rechazada: { label: "Rechazada", icon: XCircle, className: "bg-destructive/10 text-destructive" },
  cancelada: { label: "Cancelada", icon: Ban, className: "bg-border/50 text-muted" },
};

export function StateBadge({ status }: { status: RequestStatus }) {
  const { label, icon: Icon, className } = STATUS_META[status];
  return (
    <span data-testid="request-state" className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${className}`}>
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      {label}
    </span>
  );
}

/** What approving did, in words, for an approved request. */
export function resultText(r: ExpenseRequest): string | null {
  if (r.status !== "aprobada") return null;
  return r.result_kind === "gasto_futuro" ? "Se movió a gastos futuros." : "Se registró como gasto.";
}
