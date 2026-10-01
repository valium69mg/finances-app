import type { ReactNode } from "react";
import { MessageSquareText, Undo2 } from "lucide-react";
import type { ExpenseRequest } from "../../api/expenseRequests";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";
import { resultText, revertHistory, StateBadge } from "./labels";

interface Props {
  request: ExpenseRequest;
  /** The owner sees who asked; the requester already knows. */
  showRequester?: boolean;
  /** Buttons of the card (cancel for the requester, approve and reject for the owner). */
  actions?: ReactNode;
}

/** One expense request: what was asked, its state, what the owner answered and what happened. */
export function RequestCard({ request: r, showRequester = false, actions }: Props) {
  const result = resultText(r);
  return (
    <li data-testid="request-card" className="space-y-3 p-4">
      <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
        <div className="min-w-0">
          <p className="break-words font-medium">{r.description}</p>
          <p className="text-sm tabular-nums text-muted">
            {formatMoney(r.amount)} · {dateLabel(r.expense_date)}
          </p>
        </div>
        <StateBadge status={r.status} />
      </div>
      <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
        {showRequester && (
          <>
            <dt className="text-muted">Pidió</dt>
            <dd className="min-w-0 break-all">{r.requester_email}</dd>
          </>
        )}
        <dt className="text-muted">Categoría sugerida</dt>
        <dd className="min-w-0 break-words">{r.suggested_category ?? "Sin sugerencia"}</dd>
      </dl>
      {r.status === "rechazada" && r.decision_comment && (
        <p className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm">
          <MessageSquareText className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
          <span className="min-w-0 break-words">
            <span className="font-medium">Comentario: </span>
            {r.decision_comment}
          </span>
        </p>
      )}
      {result && <p className="text-sm">{result}</p>}
      {r.status === "solicitada" && r.revert_count > 0 && (
        <p data-testid="revert-history" className="flex items-start gap-2 text-sm text-muted">
          <Undo2 className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 break-words">{revertHistory(r.revert_count)}</span>
        </p>
      )}
      {actions && <div className="flex flex-wrap gap-3">{actions}</div>}
    </li>
  );
}
