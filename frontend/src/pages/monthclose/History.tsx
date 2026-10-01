import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, CalendarCheck, ChevronRight, Loader2 } from "lucide-react";
import { listMonthCloses, monthCloseKeys, type MonthClose } from "../../api/monthClose";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { periodLabel } from "../taxfiling/labels";
import { CloseDetail } from "./CloseDetail";
import { describeMonthCloseError } from "./errors";
import { closedAtLabel, overBudgetOf } from "./labels";
import { EmptyNote } from "../../components/EmptyNote";

interface Props {
  /** Period of the close shown in the detail, or null. */
  selected: string | null;
  onSelect: (period: string | null) => void;
  /** Called after a close was discarded, with its period. */
  onDeleted: (period: string) => void;
}

function Row({ close, selected, onSelect }: { close: MonthClose; selected: boolean; onSelect: () => void }) {
  const over = overBudgetOf(close).length;
  return (
    <li className={selected ? "bg-primary/5" : undefined}>
      <button
        type="button"
        onClick={onSelect}
        aria-label={`Ver cierre de ${periodLabel(close.period)}`}
        aria-current={selected ? "true" : undefined}
        className="focus-ring flex w-full flex-col gap-2 p-4 text-left transition-colors duration-200 hover:bg-primary/5 sm:flex-row sm:items-center sm:justify-between"
      >
        <span className="min-w-0">
          <span className="block break-words font-medium capitalize">{periodLabel(close.period)}</span>
          <span className="mt-0.5 block text-sm text-muted">Cerrado el {close.closed_at ? closedAtLabel(close.closed_at) : "—"}</span>
        </span>
        <span className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
          <span className="tabular-nums">
            Disponible <span className={`font-semibold ${close.available.startsWith("-") ? "text-destructive" : ""}`}>{formatMoney(close.available)}</span>
          </span>
          {over > 0 && (
            <span className="flex items-center gap-1 font-medium text-destructive">
              <AlertTriangle className="h-4 w-4" aria-hidden="true" />
              {over === 1 ? "1 categoría excedida" : `${over} categorías excedidas`}
            </span>
          )}
          <ChevronRight className="hidden h-4 w-4 text-muted sm:block" aria-hidden="true" />
        </span>
      </button>
    </li>
  );
}

/** Stored closes, newest first, with the snapshot of the selected one below the list. */
export function History({ selected, onSelect, onDeleted }: Props) {
  const list = useQuery({ queryKey: monthCloseKeys.list, queryFn: listMonthCloses, retry: false });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando cierres…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeMonthCloseError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    return <EmptyNote icon={CalendarCheck}>Aún no has cerrado ningún mes. Cuando cierres uno, su resumen se guardará aquí tal como estaba ese día.</EmptyNote>;
  }

  const open = list.data.find((c) => c.period === selected);
  return (
    <div className="space-y-6">
      <ul aria-label="Cierres guardados" className="divide-y divide-border rounded-xl border border-border">
        {list.data.map((c) => (
          <Row key={c.period} close={c} selected={c.period === selected} onSelect={() => onSelect(c.period === selected ? null : c.period)} />
        ))}
      </ul>
      {open && <CloseDetail close={open} onClose={() => onSelect(null)} onDeleted={onDeleted} />}
    </div>
  );
}
