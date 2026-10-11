import { useQuery } from "@tanstack/react-query";
import { Coins, Loader2, Wallet } from "lucide-react";
import { Link } from "react-router-dom";
import { dashboardKeys, getBudgetDashboard, getDashboard, isFullDashboard, type BudgetDashboard, type Dashboard as DashboardData, type RecentMovement } from "../api/dashboard";
import { ApiError } from "../api/client";
import { getPersonalDashboard, personalKeys } from "../api/personal";
import { useAuth } from "../auth/AuthContext";
import { TextField } from "../components/AuthCard";
import { EmptyNote } from "../components/EmptyNote";
import { IconChip } from "../components/IconChip";
import { PageTitle } from "../components/PageTitle";
import { BudgetTable, type RowLink } from "./dashboard/BudgetTable";
import { CategorySpend } from "./dashboard/CategorySpend";
import { CycleProgress } from "./dashboard/CycleProgress";
import { FutureExpensesCard } from "./dashboard/FutureExpensesCard";
import { RecentMovements } from "./dashboard/RecentMovements";
import { UpcomingBills } from "./dashboard/UpcomingBills";
import { TaxCard } from "./dashboard/TaxCard";
import { TotalsCards } from "./dashboard/TotalsCards";
import { describeDashboardError } from "./dashboard/errors";
import { useMonthParam } from "../hooks/useUrlParams";
import { linkWithMonth } from "../lib/monthParam";
import { moduleIcon } from "../modules";
import { cycleOf, rangeText } from "./cycle";
import { useCyclePeriod, useCycleStartDay } from "./useCycle";
import { EmergencyCard } from "./savings/PortfolioPanel";
import { describePersonalError } from "./personal/errors";
import { ErrorBanner, secondaryButton } from "./settings/ui";

const isEmptyMonth = (d: DashboardData) =>
  /^-?0(\.0+)?$/.test(d.income) && /^-?0(\.0+)?$/.test(d.expenses) && /^-?0(\.0+)?$/.test(d.savings);

/** Budget against spending by Gasto category: the one section every role sees. */
function BudgetSection({
  categories,
  emptyText,
  rowLink,
}: {
  categories: BudgetDashboard["categories"];
  emptyText: string;
  rowLink?: RowLink;
}) {
  return (
    <section aria-labelledby="budget-title">
      <h2 id="budget-title" className="mb-3 flex items-center gap-2 text-lg font-semibold tracking-tight">
        <IconChip icon={Wallet} tone="expense" size="sm" />
        Presupuesto por categoría
      </h2>
      {categories.length === 0 ? <EmptyNote icon={Wallet}>{emptyText}</EmptyNote> : <BudgetTable categories={categories} rowLink={rowLink} />}
    </section>
  );
}

const OWNER_EMPTY_BUDGET = "Aún no hay categorías de gasto. Agrégalas en Configuración para ver tu presupuesto.";

/** The owner's pages the Panel links to, by movement kind. */
const MOVEMENT_PAGE: Record<RecentMovement["kind"], string> = { Gasto: "/gastos", Ingreso: "/ingresos", Ahorro: "/ahorros" };
/** The savings category of the emergency fund (fixed by the backend), the target of its card. */
const EMERGENCY_FUND_CATEGORY = "Fondo de emergencia";

/**
 * Interactive owner Panel: every link carries the month the figures are for (`dashboard.month`, the label the server
 * answered) as `?mes=`. Targets: the three totals cards, Progreso del ciclo and a budget row go to Gastos (a row
 * filtered by its category), Fondo de emergencia to Ahorros filtered by its category, ISR RESICO to Declaración
 * (`?period=`, the page's own parameter), a recent movement to its page in that movement's cycle with `?editar=<id>`
 * (its edit form), an upcoming bill to Pagos recurrentes with `?pago=<id>` (its "Pagar" dialog) and a future expense
 * to Gastos futuros with `?editar=<id>` (its edit form). Disponible has no link.
 */
function DashboardBody({ dashboard, startDay }: { dashboard: DashboardData; startDay: number }) {
  const month = dashboard.month;
  // A recent movement is not always in the displayed cycle: its own cycle is the month its page must open on.
  const movementLink = (m: RecentMovement) => {
    const inPeriod = m.date >= dashboard.period_start && m.date <= dashboard.period_end;
    return linkWithMonth(MOVEMENT_PAGE[m.kind], inPeriod ? month : cycleOf(m.date, startDay), { editar: m.id });
  };
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
        <TotalsCards
          dashboard={dashboard}
          links={{ income: linkWithMonth("/ingresos", month), expenses: linkWithMonth("/gastos", month), savings: linkWithMonth("/ahorros", month) }}
        />
      </section>

      <div className="grid gap-4 md:grid-cols-2">
        <CycleProgress dashboard={dashboard} to={linkWithMonth("/gastos", month)} />
        <UpcomingBills bills={dashboard.upcoming_bills} linkFor={(b) => linkWithMonth("/pagos-recurrentes", "", { pago: b.id })} />
      </div>

      <BudgetSection
        categories={dashboard.categories}
        emptyText={OWNER_EMPTY_BUDGET}
        rowLink={(c) => linkWithMonth("/gastos", month, { categoria: c.category })}
      />

      <div className="grid gap-4 md:grid-cols-2">
        <FutureExpensesCard future={dashboard.future_expenses} linkFor={(f) => linkWithMonth("/gastos-futuros", "", { editar: f.id })} />
        <RecentMovements movements={dashboard.recent_movements} linkFor={movementLink} />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <EmergencyCard emergency={dashboard.emergency} to={linkWithMonth("/ahorros", month, { categoria: EMERGENCY_FUND_CATEGORY })} />
        <TaxCard tax={dashboard.tax} to={`/declaracion?period=${month}`} />
      </div>
    </div>
  );
}

/** Main screen: the household role only gets the budget by category; everyone else the full dashboard. */
export function Dashboard() {
  const { role } = useAuth();
  return role === "household" ? <HouseholdDashboard /> : <OwnerDashboard />;
}

/** The household Panel's targets: every one is a page her role can open. */
const FAMILY_BUDGET_PATH = "/peticiones";

/**
 * Household view: her own summary of the cycle on top (interactive: every card and row links to the page behind it,
 * carrying the month) and the family budget by category below, exactly as before. The month lives in the URL
 * (`?mes=YYYY-MM`, empty = the current cycle, which the server resolves with the owner's cut day), so a link back
 * to the Panel or a reload keeps it. She cannot read the settings: the range hint comes from her dashboard payload.
 */
function HouseholdDashboard() {
  const [picked, setPicked] = useMonthParam();
  const mine = useQuery({ queryKey: personalKeys.dashboard(picked), queryFn: () => getPersonalDashboard(picked), retry: false });
  const budget = useQuery({
    queryKey: dashboardKeys.budget(picked),
    queryFn: () => getBudgetDashboard(picked),
    retry: false,
  });
  const month = picked || mine.data?.month || budget.data?.month || "";
  const range = mine.data ?? budget.data;
  const hint = range?.period_start ? rangeText(range.period_start, range.period_end) : undefined;
  // Links carry the month the figures are for (the server's own label when none was picked).
  const linkMonth = mine.data?.month ?? picked;

  return (
    <section aria-labelledby="page-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <PageTitle icon={moduleIcon("/")}>Panel</PageTitle>
          <p className="mt-1 text-sm text-muted">Tu resumen del periodo y el presupuesto familiar por categoría.</p>
        </div>
        <div className="w-full min-w-0 max-w-full sm:w-56">
          <TextField label="Mes" type="month" hint={hint} value={month} onChange={(e) => e.target.value && setPicked(e.target.value)} />
        </div>
      </div>

      <div className="mt-6 space-y-6">
        <section aria-labelledby="mine-title">
          <h2 id="mine-title" className="mb-3 flex items-center gap-2 text-lg font-semibold tracking-tight">
            <IconChip icon={Coins} size="sm" />
            Mi resumen
          </h2>
          {mine.isPending && (
            <p role="status" className="flex items-center gap-2 text-sm text-muted">
              <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              Cargando tu resumen…
            </p>
          )}
          {mine.isError && (
            <div className="space-y-3">
              <ErrorBanner>{describePersonalError(mine.error)}</ErrorBanner>
              <button type="button" onClick={() => void mine.refetch()} className={secondaryButton}>
                Reintentar
              </button>
            </div>
          )}
          {mine.data && (
            <div className="space-y-4">
              <TotalsCards
                dashboard={mine.data}
                links={{
                  income: linkWithMonth("/mis-ingresos", linkMonth),
                  expenses: linkWithMonth("/mis-gastos", linkMonth),
                  savings: linkWithMonth("/mis-ahorros", linkMonth),
                }}
              />
              <CategorySpend
                categories={mine.data.categories}
                total={mine.data.expenses}
                linkFor={(c) => linkWithMonth("/mis-gastos", linkMonth, { categoria: c.category_id })}
              />
            </div>
          )}
        </section>

        <div>
          {budget.isPending && (
            <p role="status" className="flex items-center gap-2 text-sm text-muted">
              <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              Cargando presupuesto…
            </p>
          )}
          {budget.isError && (
            <div className="space-y-3">
              <ErrorBanner>{describeDashboardError(budget.error)}</ErrorBanner>
              <button type="button" onClick={() => void budget.refetch()} className={secondaryButton}>
                Reintentar
              </button>
            </div>
          )}
          {budget.data && (
            <BudgetSection
              categories={budget.data.categories}
              emptyText="Aún no hay categorías con presupuesto en este periodo."
              rowLink={() => FAMILY_BUDGET_PATH}
            />
          )}
        </div>
      </div>
    </section>
  );
}

/** Owner view: the month's budget versus actual, totals, emergency fund and estimated ISR. */
function OwnerDashboard() {
  const { month, setMonth, rangeHint } = useCyclePeriod();
  const { startDay } = useCycleStartDay();
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
          (isFullDashboard(query.data) ? <DashboardBody dashboard={query.data} startDay={startDay} /> : <BudgetSection categories={query.data.categories} emptyText={OWNER_EMPTY_BUDGET} />)}
      </div>
    </section>
  );
}
