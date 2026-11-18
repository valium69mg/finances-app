import { useId } from "react";
import { Landmark, Loader2, PiggyBank, ShieldCheck, TrendingUp } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { CardTitle } from "../../components/CardTitle";
import { EmptyNote } from "../../components/EmptyNote";
import { StatCard, cardClass } from "../../components/LinkCard";
import { getPortfolio, savingsKeys, type EmergencyProgress, type Portfolio } from "../../api/savings";
import { formatMoney, percentOf } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";

/** The per-instrument portfolio as dashboard cards: the instrument's name and what it is worth now. */
function PortfolioCards({ portfolio }: { portfolio: Portfolio }) {
  return (
    <dl aria-label="Instrumentos del portafolio" className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 lg:grid-cols-4">
      {portfolio.rows.map((r) => (
        <StatCard key={r.id} testId="instrument-card" label={r.name} icon={Landmark} tone="saving" hint={r.unvalued ? "sin valuar" : undefined}>
          {r.unvalued ? "—" : formatMoney(r.value)}
        </StatCard>
      ))}
    </dl>
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
export function EmergencyCard({ emergency, to }: { emergency: EmergencyProgress; to?: string }) {
  const pct = percentOf(emergency.accumulated, emergency.goal);
  const hasGoal = /[1-9]/.test(emergency.goal);
  // Unique per instance: a close preview and a stored close can be on screen together.
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className={cardClass(to)}>
      <CardTitle id={titleId} icon={ShieldCheck} tone="saving" to={to}>
        Fondo de emergencia
      </CardTitle>
      {hasGoal ? (
        <>
          <p className="mt-3 text-sm tabular-nums">
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
            <div className="h-full rounded-full bg-saving" style={{ width: `${pct}%` }} />
          </div>
          {pct >= 100 && <p className="mt-2 text-sm font-medium">Meta alcanzada.</p>}
        </>
      ) : (
        <EmptyNote icon={ShieldCheck}>Aún no hay una meta definida. Configura tus meses de fondo de emergencia y tus presupuestos en Configuración.</EmptyNote>
      )}
    </section>
  );
}

/**
 * Headline figures: what the portfolio is worth now and what was put in, side by side at every width (like the
 * dashboard KPIs), with the emergency fund progress full width below them.
 */
function Headline({ portfolio }: { portfolio: Portfolio }) {
  return (
    <div className="space-y-4">
      <dl aria-label="Resumen del portafolio" className="grid grid-cols-2 gap-3 sm:gap-4">
        <StatCard label="Valor del portafolio" hint="Según la última valuación de cada instrumento" icon={TrendingUp} tone="saving">
          {formatMoney(portfolio.total_value)}
        </StatCard>
        <StatCard label="Total aportado" icon={PiggyBank} tone="saving">
          {formatMoney(portfolio.total_contributed)}
        </StatCard>
      </dl>
      <EmergencyCard emergency={portfolio.emergency} />
    </div>
  );
}

/**
 * Savings summary: headline figures and the emergency fund, the subtotals by type and destination, and the portfolio
 * per instrument as cards.
 */
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
      <Headline portfolio={p} />
      <Subtotals portfolio={p} />
      <section aria-labelledby="by-instrument-title" className="space-y-2">
        <h3 id="by-instrument-title" className="text-base font-semibold tracking-tight">
          Por instrumento
        </h3>
        <PortfolioCards portfolio={p} />
      </section>
    </div>
  );
}
