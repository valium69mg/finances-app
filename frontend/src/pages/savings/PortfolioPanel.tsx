import { Loader2 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { getPortfolio, savingsKeys, type EmergencyProgress, type Portfolio } from "../../api/savings";
import { formatMoney, formatPercent, percentOf } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";

const th = "px-3 py-2 text-left text-sm font-medium text-muted";
const thNum = "px-3 py-2 text-right text-sm font-medium text-muted";
const td = "px-3 py-2 align-top";
const tdNum = "px-3 py-2 text-right align-top tabular-nums";

function PortfolioTable({ portfolio }: { portfolio: Portfolio }) {
  return (
    <div className="overflow-x-auto rounded-xl border border-border">
      <table className="w-full min-w-[44rem] border-collapse text-sm">
        <caption className="sr-only">Portafolio por instrumento</caption>
        <thead>
          <tr className="border-b border-border">
            <th scope="col" className={th}>
              Instrumento
            </th>
            <th scope="col" className={thNum}>
              Aportado
            </th>
            <th scope="col" className={thNum}>
              Valor actual
            </th>
            <th scope="col" className={th}>
              Valuación
            </th>
            <th scope="col" className={thNum}>
              Ganancia
            </th>
            <th scope="col" className={thNum}>
              Ganancia %
            </th>
            <th scope="col" className={thNum}>
              % del total
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {portfolio.rows.map((r) => (
            <tr key={r.id}>
              <th scope="row" className={`${td} text-left font-medium`}>
                <span className="break-words">{r.name}</span>
                <span className="block break-words text-xs font-normal text-muted">
                  {r.type}
                  {r.platform ? ` · ${r.platform}` : ""}
                </span>
              </th>
              <td className={tdNum}>{formatMoney(r.contributed)}</td>
              <td className={tdNum}>{r.unvalued ? "—" : formatMoney(r.value)}</td>
              <td className={td}>{r.unvalued || !r.value_date ? <span className="text-muted">sin valuar</span> : r.value_date}</td>
              <td className={tdNum}>{r.unvalued ? "—" : formatMoney(r.gain)}</td>
              <td className={tdNum}>{r.unvalued ? "—" : formatPercent(r.gain_pct)}</td>
              <td className={tdNum}>{r.unvalued ? "—" : formatPercent(r.pct_of_total)}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr className="border-t border-border font-semibold">
            <th scope="row" className={`${td} text-left`}>
              Total
            </th>
            <td className={tdNum}>{formatMoney(portfolio.total_contributed)}</td>
            <td className={tdNum}>{formatMoney(portfolio.total_value)}</td>
            <td className={td} />
            <td className={td} />
            <td className={td} />
            <td className={td} />
          </tr>
        </tfoot>
      </table>
    </div>
  );
}

function Subtotals({ portfolio }: { portfolio: Portfolio }) {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <section aria-labelledby="by-type-title" className="rounded-lg border border-border p-4">
        <h3 id="by-type-title" className="font-semibold">
          Subtotales por tipo
        </h3>
        {portfolio.by_type.length === 0 ? (
          <p className="mt-2 text-sm text-muted">Sin datos por tipo.</p>
        ) : (
          <ul className="mt-2 divide-y divide-border text-sm">
            {portfolio.by_type.map((t) => (
              <li key={t.type} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 py-1.5">
                <span className="min-w-0 break-words font-medium">{t.type}</span>
                <span className="min-w-0 break-words tabular-nums">
                  {formatMoney(t.value)} <span className="text-muted">de {formatMoney(t.contributed)} aportados</span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section aria-labelledby="by-destination-title" className="rounded-lg border border-border p-4">
        <h3 id="by-destination-title" className="font-semibold">
          Subtotales por destino
        </h3>
        {portfolio.by_destination.length === 0 ? (
          <p className="mt-2 text-sm text-muted">Sin datos por destino.</p>
        ) : (
          <ul className="mt-2 divide-y divide-border text-sm">
            {portfolio.by_destination.map((d) => (
              <li key={d.category} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 py-1.5">
                <span className="min-w-0 break-words font-medium">{d.category}</span>
                <span className="tabular-nums">{formatMoney(d.balance)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

/** Emergency fund progress: the accumulated balance against its goal (also used by the dashboard). */
export function EmergencyCard({ emergency }: { emergency: EmergencyProgress }) {
  const pct = percentOf(emergency.accumulated, emergency.goal);
  const hasGoal = /[1-9]/.test(emergency.goal);
  return (
    <section aria-labelledby="emergency-title" className="rounded-lg border border-border p-4">
      <h3 id="emergency-title" className="font-semibold">
        Fondo de emergencia
      </h3>
      {hasGoal ? (
        <>
          <p className="mt-1 text-sm tabular-nums">
            {formatMoney(emergency.accumulated)} de {formatMoney(emergency.goal)} ({pct.toFixed(2)}%)
          </p>
          <div
            role="progressbar"
            aria-label="Avance del fondo de emergencia"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(pct)}
            className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-border"
          >
            <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
          </div>
          {pct >= 100 && <p className="mt-2 text-sm font-medium">Meta alcanzada.</p>}
        </>
      ) : (
        <p className="mt-1 text-sm text-muted">
          Aún no hay una meta definida. Configura tus meses de fondo de emergencia y tus presupuestos en Configuración.
        </p>
      )}
    </section>
  );
}

/** Portfolio per instrument with totals, subtotals and the emergency fund progress. */
export function PortfolioPanel() {
  const portfolio = useQuery({ queryKey: savingsKeys.portfolio, queryFn: getPortfolio, retry: false });

  if (portfolio.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando portafolio…
      </p>
    );
  }
  if (portfolio.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeSavingsError(portfolio.error)}</ErrorBanner>
        <button type="button" onClick={() => void portfolio.refetch()} className={secondaryButton}>
          Reintentar
        </button>
      </div>
    );
  }
  const p = portfolio.data;
  if (p.rows.length === 0) {
    return <p className="text-sm text-muted">Aún no hay instrumentos. Agrégalos en Configuración para ver tu portafolio.</p>;
  }
  return (
    <div className="space-y-4">
      <PortfolioTable portfolio={p} />
      <Subtotals portfolio={p} />
      <EmergencyCard emergency={p.emergency} />
    </div>
  );
}
