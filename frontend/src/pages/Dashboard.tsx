import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Link } from "react-router-dom";
import { dashboardKeys, getDashboard, type Dashboard as DashboardData } from "../api/dashboard";
import { ApiError } from "../api/client";
import { TextField } from "../components/AuthCard";
import { BudgetTable } from "./dashboard/BudgetTable";
import { CycleProgress } from "./dashboard/CycleProgress";
import { FutureExpensesCard } from "./dashboard/FutureExpensesCard";
import { RecentMovements } from "./dashboard/RecentMovements";
import { UpcomingBills } from "./dashboard/UpcomingBills";
import { TaxCard } from "./dashboard/TaxCard";
import { TotalsCards } from "./dashboard/TotalsCards";
import { describeDashboardError } from "./dashboard/errors";
import { rangeText } from "./cycle";
import { useCyclePeriod } from "./useCycle";
import { EmergencyCard } from "./savings/PortfolioPanel";
import { ErrorBanner, secondaryButton } from "./settings/ui";

const isEmptyMonth = (d: DashboardData) =>
  /^-?0(\.0+)?$/.test(d.income) && /^-?0(\.0+)?$/.test(d.expenses) && /^-?0(\.0+)?$/.test(d.savings);

function DashboardBody({ dashboard }: { dashboard: DashboardData }) {
  return (
    <div className="space-y-6">
      {isEmptyMonth(dashboard) && (
        <p className="rounded-lg border border-border p-4 text-sm text-muted">
          Aún no hay movimientos en este periodo. Registra ingresos, gastos o ahorros y aquí verás tu resumen.
        </p>
      )}

      <section aria-labelledby="totals-title">
        <h2 id="totals-title" className="mb-3 text-lg font-semibold tracking-tight">
          Resumen del mes
        </h2>
        <TotalsCards dashboard={dashboard} />
      </section>

      <div className="grid gap-4 md:grid-cols-2">
        <CycleProgress dashboard={dashboard} />
        <UpcomingBills bills={dashboard.upcoming_bills} />
      </div>

      <section aria-labelledby="budget-title">
        <h2 id="budget-title" className="mb-3 text-lg font-semibold tracking-tight">
          Presupuesto por categoría
        </h2>
        {dashboard.categories.length === 0 ? (
          <p className="text-sm text-muted">Aún no hay categorías de gasto. Agrégalas en Configuración para ver tu presupuesto.</p>
        ) : (
          <BudgetTable categories={dashboard.categories} />
        )}
      </section>

      <div className="grid gap-4 md:grid-cols-2">
        <FutureExpensesCard future={dashboard.future_expenses} />
        <RecentMovements movements={dashboard.recent_movements} />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <EmergencyCard emergency={dashboard.emergency} />
        <TaxCard tax={dashboard.tax} />
      </div>
    </div>
  );
}

/** Main screen: the month's budget versus actual, totals, emergency fund and estimated ISR. */
export function Dashboard() {
  const { month, setMonth, rangeHint } = useCyclePeriod();
  const query = useQuery({
    queryKey: dashboardKeys.month(month ?? ""),
    queryFn: () => getDashboard(month ?? ""),
    enabled: month !== null,
    retry: false,
  });
  // The API's own period wins once it answers for the selected month.
  const hint = query.data && query.data.month === month && query.data.period_start ? rangeText(query.data.period_start, query.data.period_end) : rangeHint;
  const incomplete = query.error instanceof ApiError && query.error.code === "settings_incomplete";

  return (
    <section aria-labelledby="page-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
            Panel
          </h1>
          <p className="mt-1 text-sm text-muted">Tu resumen del mes: presupuesto contra gasto real, dinero disponible y ahorro.</p>
        </div>
        <div className="w-full min-w-0 max-w-full sm:w-56">
          <TextField label="Mes" type="month" hint={hint || undefined} value={month ?? ""} onChange={(e) => e.target.value && setMonth(e.target.value)} />
        </div>
      </div>

      <div className="mt-6">
        {query.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando resumen…
          </p>
        )}
        {query.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeDashboardError(query.error)}</ErrorBanner>
            <div className="flex flex-wrap items-center gap-3">
              <button type="button" onClick={() => void query.refetch()} className={secondaryButton}>
                Reintentar
              </button>
              {incomplete && (
                <Link to="/configuracion" className="text-sm font-medium underline">
                  Ir a Configuración
                </Link>
              )}
            </div>
          </div>
        )}
        {query.data && <DashboardBody dashboard={query.data} />}
      </div>
    </section>
  );
}
