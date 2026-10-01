import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Coins, Loader2, Wallet } from "lucide-react";
import { Link } from "react-router-dom";
import { dashboardKeys, getBudgetDashboard, getDashboard, isFullDashboard, type BudgetDashboard, type Dashboard as DashboardData } from "../api/dashboard";
import { ApiError } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { TextField } from "../components/AuthCard";
import { EmptyNote } from "../components/EmptyNote";
import { IconChip } from "../components/IconChip";
import { PageTitle } from "../components/PageTitle";
import { BudgetTable } from "./dashboard/BudgetTable";
import { CycleProgress } from "./dashboard/CycleProgress";
import { FutureExpensesCard } from "./dashboard/FutureExpensesCard";
import { RecentMovements } from "./dashboard/RecentMovements";
import { UpcomingBills } from "./dashboard/UpcomingBills";
import { TaxCard } from "./dashboard/TaxCard";
import { TotalsCards } from "./dashboard/TotalsCards";
import { describeDashboardError } from "./dashboard/errors";
import { moduleIcon } from "../modules";
import { rangeText } from "./cycle";
import { useCyclePeriod } from "./useCycle";
import { EmergencyCard } from "./savings/PortfolioPanel";
import { ErrorBanner, secondaryButton } from "./settings/ui";

const isEmptyMonth = (d: DashboardData) =>
  /^-?0(\.0+)?$/.test(d.income) && /^-?0(\.0+)?$/.test(d.expenses) && /^-?0(\.0+)?$/.test(d.savings);

/** Budget against spending by Gasto category: the one section every role sees. */
function BudgetSection({ categories, emptyText }: { categories: BudgetDashboard["categories"]; emptyText: string }) {
  return (
    <section aria-labelledby="budget-title">
      <h2 id="budget-title" className="mb-3 flex items-center gap-2 text-lg font-semibold tracking-tight">
        <IconChip icon={Wallet} tone="expense" size="sm" />
        Presupuesto por categoría
      </h2>
      {categories.length === 0 ? <EmptyNote icon={Wallet}>{emptyText}</EmptyNote> : <BudgetTable categories={categories} />}
    </section>
  );
}

const OWNER_EMPTY_BUDGET = "Aún no hay categorías de gasto. Agrégalas en Configuración para ver tu presupuesto.";

function DashboardBody({ dashboard }: { dashboard: DashboardData }) {
  return (
    <div className="space-y-6">
      {isEmptyMonth(dashboard) && (
        <p className="rounded-lg border border-border p-4 text-sm text-muted">
          Aún no hay movimientos en este periodo. Registra ingresos, gastos o ahorros y aquí verás tu resumen.
        </p>
      )}

      <section aria-labelledby="totals-title">
        <h2 id="totals-title" className="mb-3 flex items-center gap-2 text-lg font-semibold tracking-tight">
          <IconChip icon={Coins} size="sm" />
          Resumen del mes
        </h2>
        <TotalsCards dashboard={dashboard} />
      </section>

      <div className="grid gap-4 md:grid-cols-2">
        <CycleProgress dashboard={dashboard} />
        <UpcomingBills bills={dashboard.upcoming_bills} />
      </div>

      <BudgetSection categories={dashboard.categories} emptyText={OWNER_EMPTY_BUDGET} />

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

/** Main screen: the household role only gets the budget by category; everyone else the full dashboard. */
export function Dashboard() {
  const { role } = useAuth();
  return role === "household" ? <HouseholdDashboard /> : <OwnerDashboard />;
}

/**
 * Household view: only the budget by category. It asks the server for the current cycle (an empty month) and
 * never reads the settings, which this role cannot open, so the page does not depend on anything but its payload.
 */
function HouseholdDashboard() {
  const [picked, setPicked] = useState<string | null>(null);
  const query = useQuery({
    queryKey: dashboardKeys.budget(picked ?? ""),
    queryFn: () => getBudgetDashboard(picked ?? ""),
    retry: false,
  });
  const month = picked ?? query.data?.month ?? "";
  const hint = query.data?.period_start ? rangeText(query.data.period_start, query.data.period_end) : undefined;

  return (
    <section aria-labelledby="page-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <PageTitle icon={moduleIcon("/")}>Panel</PageTitle>
          <p className="mt-1 text-sm text-muted">El presupuesto del mes: lo que se ha gastado en cada categoría contra lo presupuestado.</p>
        </div>
        <div className="w-full min-w-0 max-w-full sm:w-56">
          <TextField label="Mes" type="month" hint={hint} value={month} onChange={(e) => e.target.value && setPicked(e.target.value)} />
        </div>
      </div>

      <div className="mt-6">
        {query.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando presupuesto…
          </p>
        )}
        {query.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeDashboardError(query.error)}</ErrorBanner>
            <button type="button" onClick={() => void query.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {query.data && <BudgetSection categories={query.data.categories} emptyText="Aún no hay categorías con presupuesto en este periodo." />}
      </div>
    </section>
  );
}

/** Owner view: the month's budget versus actual, totals, emergency fund and estimated ISR. */
function OwnerDashboard() {
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
          <PageTitle icon={moduleIcon("/")}>Panel</PageTitle>
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
        {query.data &&
          // The server decides what a session may see: a reduced payload (the role changed since the last
          // refresh) shows the budget only instead of breaking on figures that are not there.
          (isFullDashboard(query.data) ? <DashboardBody dashboard={query.data} /> : <BudgetSection categories={query.data.categories} emptyText={OWNER_EMPTY_BUDGET} />)}
      </div>
    </section>
  );
}
