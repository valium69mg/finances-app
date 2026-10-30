import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Loader2 } from "lucide-react";
import { expensesKeys, listExpenses, type Expense } from "../../api/expenses";
import { ErrorBanner, rowButton, secondaryButton } from "../settings/ui";
import { describeExpenseError } from "./errors";
import { formatMoney } from "./money";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND } from "../../lib/tones";

interface Props {
  month: string;
  /** Category the list is narrowed to (client side: the list endpoint has no category filter). */
  category?: string;
  /** A category filter from the URL is still being validated: show the loading state instead of every row. */
  waiting?: boolean;
  editingId: number | null;
  onEdit: (expense: Expense) => void;
}

export function ExpenseList({ month, category = "", waiting = false, editingId, onEdit }: Props) {
  const list = useQuery({ queryKey: expensesKeys.list(month), queryFn: () => listExpenses(month), retry: false });

  if (list.isPending || waiting) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando gastos…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeExpenseError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  const rows = category ? list.data.filter((e) => e.category === category) : list.data;
  if (rows.length === 0) {
    return (
      <EmptyNote icon={KIND.Gasto.icon}>
        {category ? `No hay gastos de ${category} en este periodo.` : "No hay gastos registrados en este periodo."}
      </EmptyNote>
    );
  }

  return (
    <ul className="space-y-2">
      {rows.map((e) => {
        const label = e.description || "Sin descripción";
        return (
          <li key={e.id}>
            <button type="button" onClick={() => onEdit(e)} aria-label={`Editar ${label}`} className={`${rowButton} ${editingId === e.id ? "bg-primary/5" : ""}`}>
              <span className="min-w-0 flex-1">
                <span className="block break-words font-medium">{label}</span>
                <span className="mt-0.5 block break-words text-sm text-muted">
                  {e.date} · {e.category || "Sin categoría"} · {e.payment_method}
                </span>
              </span>
              <span className="shrink-0 text-right">
                <span className="block font-semibold tabular-nums">
                  {formatMoney(e.amount)} {e.currency}
                </span>
                {e.currency !== "MXN" && (
                  <span className="block text-sm tabular-nums text-muted">
                    ≈ {formatMoney(e.amount_mxn)} MXN{e.exchange_rate ? ` (TC ${e.exchange_rate})` : ""}
                  </span>
                )}
              </span>
              <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
            </button>
          </li>
        );
      })}
    </ul>
  );
}
