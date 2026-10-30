import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Loader2 } from "lucide-react";
import { SAVINGS_LIST_LIMIT, listSavings, savingsKeys, type Saving } from "../../api/savings";
import type { Instrument } from "../../api/settings";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND } from "../../lib/tones";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, rowButton, secondaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";

interface Props {
  month: string;
  /** Category the list is narrowed to (client side: the list endpoint has no category filter). */
  category?: string;
  /** A category filter from the URL is still being validated: show the loading state instead of every row. */
  waiting?: boolean;
  instruments: Instrument[];
  /** Opens a saving: its edit dialog, or for a transfer leg a dialog whose only action is deleting the transfer. */
  onOpen: (saving: Saving) => void;
}

/** The month's savings as rows; each row is one button that opens its dialog (edit and delete live there). */
export function SavingsList({ month, category = "", waiting = false, instruments, onOpen }: Props) {
  const list = useQuery({ queryKey: savingsKeys.list(month), queryFn: () => listSavings(month), retry: false });

  if (list.isPending || waiting) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando ahorros…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeSavingsError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  const rows = category ? list.data.filter((s) => s.category === category) : list.data;
  if (rows.length === 0) {
    return (
      <EmptyNote icon={KIND.Ahorro.icon}>
        {category ? `No hay ahorros de ${category} en este periodo.` : "No hay ahorros registrados en este periodo."}
      </EmptyNote>
    );
  }

  return (
    <div className="space-y-3">
      {list.data.length >= SAVINGS_LIST_LIMIT && (
        <p role="status" className="text-sm text-muted">
          Mostrando los {SAVINGS_LIST_LIMIT} más recientes de este periodo; los anteriores no aparecen en la lista.
        </p>
      )}
      <ul className="space-y-2">
        {rows.map((s) => {
          const label = s.description || "Sin descripción";
          const instrument = instruments.find((i) => i.id === s.instrument)?.name ?? s.instrument;
          const withdrawal = s.amount.trim().startsWith("-");
          const isTransfer = Boolean(s.transfer_id);
          return (
            <li key={s.id}>
              <button type="button" onClick={() => onOpen(s)} aria-label={`${isTransfer ? "Ver" : "Editar"} ${label}`} className={rowButton}>
                <span className="min-w-0 flex-1">
                  <span className="block break-words font-medium">
                    {label}
                    {isTransfer && (
                      <span className="ml-2 inline-block rounded-full border border-border px-2 py-0.5 align-middle text-xs font-medium text-muted">Traspaso</span>
                    )}
                  </span>
                  <span className="mt-0.5 block break-words text-sm text-muted">
                    {s.date} · {instrument || "Sin instrumento"} · {s.category || "Sin categoría"}
                  </span>
                </span>
                <span className="shrink-0 text-right">
                  <span className={`block font-semibold tabular-nums ${withdrawal ? "text-destructive" : ""}`}>
                    {formatMoney(s.amount)} {s.currency}
                  </span>
                  {withdrawal && <span className="block text-sm text-muted">Retiro</span>}
                  {s.currency !== "MXN" && (
                    <span className="block text-sm tabular-nums text-muted">
                      ≈ {formatMoney(s.amount_mxn)} MXN{s.exchange_rate ? ` (TC ${s.exchange_rate})` : ""}
                    </span>
                  )}
                </span>
                <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
