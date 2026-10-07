import { useQuery } from "@tanstack/react-query";
import { Eye, Loader2, Wallet } from "lucide-react";
import { listTaxFilings, taxFilingKeys, type Filing, type FilingFilter } from "../../api/taxFiling";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { DOCUMENT_LABEL } from "./documents";
import { describeTaxFilingError } from "./errors";
import { periodLabel } from "./labels";
import { FilingStatusBadge } from "./StatusBadge";

const th = "px-3 py-2 text-left text-sm font-medium text-muted";
const thNum = "px-3 py-2 text-right text-sm font-medium text-muted";
const td = "px-3 py-2 align-top";
const tdNum = "px-3 py-2 text-right align-top tabular-nums";

/** IVA balance for the table: a negative balance is shown as "A favor". */
function ivaCell(due: string) {
  return due.trim().startsWith("-") ? `A favor ${formatMoney(due.trim().slice(1))}` : formatMoney(due);
}

interface Props {
  filter: FilingFilter;
  selectedPeriod: string | null;
  onSelect: (period: string) => void;
  onPay: (filing: Filing) => void;
}

/** History of registered filings with their payment status, newest period first. */
export function FilingHistory({ filter, selectedPeriod, onSelect, onPay }: Props) {
  const list = useQuery({ queryKey: taxFilingKeys.list(filter), queryFn: () => listTaxFilings(filter), retry: false });

  if (list.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando declaraciones…
      </p>
    );
  }
  if (list.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeTaxFilingError(list.error)}</ErrorBanner>
        <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  if (list.data.length === 0) {
    const filtered = Boolean(filter.year || filter.status);
    return (
      <p className="text-sm text-muted">
        {filtered ? "No hay declaraciones con estos filtros." : "Aún no hay declaraciones registradas. Calcula y registra la primera en la pestaña Declaración."}
      </p>
    );
  }

  return (
    <div className="overflow-x-auto rounded-xl border border-border">
      <table className="w-full min-w-[60rem] border-collapse text-sm">
        <caption className="sr-only">Historial de declaraciones presentadas</caption>
        <thead>
          <tr className="border-b border-border">
            <th scope="col" className={th}>
              Periodo
            </th>
            <th scope="col" className={th}>
              Estado
            </th>
            <th scope="col" className={th}>
              Presentada
            </th>
            <th scope="col" className={th}>
              Folio
            </th>
            <th scope="col" className={thNum}>
              ISR a pagar
            </th>
            <th scope="col" className={thNum}>
              IVA a pagar
            </th>
            <th scope="col" className={thNum}>
              Pagado
            </th>
            <th scope="col" className={th}>
              Documentos
            </th>
            <th scope="col" className={th}>
              Acciones
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {list.data.map((f) => (
            <tr key={f.period} className={selectedPeriod === f.period ? "bg-primary/5" : undefined}>
              <th scope="row" className={`${td} text-left font-medium`}>
                <span className="capitalize">{periodLabel(f.period)}</span>
                <span className="block text-xs font-normal text-muted">{f.period}</span>
              </th>
              <td className={td}>
                <FilingStatusBadge status={f.status} />
              </td>
              <td className={td}>{f.filing_date}</td>
              <td className={`${td} break-all`}>{f.folio || <span className="text-muted">—</span>}</td>
              <td className={tdNum}>{formatMoney(f.isr_due)}</td>
              <td className={tdNum}>{ivaCell(f.iva_due)}</td>
              <td className={tdNum}>{f.payment ? formatMoney(f.payment.total_paid) : <span className="text-muted">—</span>}</td>
              <td className={td}>
                {f.documents.length > 0 ? (
                  <span>{f.documents.map((d) => DOCUMENT_LABEL[d.kind]).join(", ")}</span>
                ) : (
                  <span className="text-muted">—</span>
                )}
              </td>
              <td className={td}>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    onClick={() => onSelect(f.period)}
                    aria-label={`Ver declaración de ${periodLabel(f.period)}`}
                    aria-current={selectedPeriod === f.period ? "true" : undefined}
                    className={secondaryButton}
                  >
                    <Eye className="h-4 w-4" aria-hidden="true" />
                    Ver detalle
                  </button>
                  {f.status === "pendiente" && (
                    <button type="button" onClick={() => onPay(f)} aria-label={`Registrar pago de ${periodLabel(f.period)}`} className={secondaryButton}>
                      <Wallet className="h-4 w-4" aria-hidden="true" />
                      Registrar pago
                    </button>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
