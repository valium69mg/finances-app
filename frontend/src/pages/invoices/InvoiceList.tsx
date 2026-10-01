import { useQuery } from "@tanstack/react-query";
import { Eye, FileText, Loader2 } from "lucide-react";
import { EmptyNote } from "../../components/EmptyNote";
import { invoiceKeys, listInvoices, type InvoiceFilter } from "../../api/invoices";
import type { Client } from "../../api/settings";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeInvoiceError } from "./errors";
import { clientName } from "./labels";
import { StateBadge } from "./StateBadge";

interface Props {
  filter: InvoiceFilter;
  clients: Client[];
  selectedId: number | null;
  onSelect: (id: number) => void;
}

export function InvoiceList({ filter, clients, selectedId, onSelect }: Props) {
  const list = useQuery({ queryKey: invoiceKeys.list(filter), queryFn: () => listInvoices(filter), retry: false });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando facturas…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeInvoiceError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    const filtered = Boolean(filter.period || filter.state);
    return (
      <EmptyNote icon={FileText}>{filtered ? "No hay facturas con estos filtros." : "Aún no hay facturas. Prepara la primera con el formulario de arriba."}</EmptyNote>
    );
  }

  return (
    <ul className="divide-y divide-border rounded-xl border border-border">
      {list.data.map((inv) => (
        <li key={inv.id} className={`flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between ${selectedId === inv.id ? "bg-primary/5" : ""}`}>
          <div className="min-w-0 flex-1">
            <p className="flex flex-wrap items-center gap-2 font-medium">
              <span>Factura #{inv.id}</span>
              <StateBadge state={inv.state} />
            </p>
            <p className="mt-0.5 break-words text-sm text-muted">
              {clientName(clients, inv.client_id)} · {inv.collection_date} · periodo {inv.period}
            </p>
            {inv.uuid && <p className="mt-0.5 break-all text-xs text-muted">UUID {inv.uuid}</p>}
          </div>
          <div className="shrink-0 text-left sm:text-right">
            <p className="font-semibold tabular-nums">
              {formatMoney(inv.total)} {inv.currency}
            </p>
            {inv.currency !== "MXN" && <p className="text-sm tabular-nums text-muted">≈ {formatMoney(inv.expected_deposit_mxn)} MXN</p>}
          </div>
          <button
            type="button"
            onClick={() => onSelect(inv.id)}
            aria-label={`Ver factura #${inv.id}`}
            aria-current={selectedId === inv.id ? "true" : undefined}
            className={`${secondaryButton} shrink-0`}
          >
            <Eye className="h-4 w-4" aria-hidden="true" />
            Ver detalle
          </button>
        </li>
      ))}
    </ul>
  );
}
