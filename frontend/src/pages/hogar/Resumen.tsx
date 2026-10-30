import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { getPersonalDashboard, personalKeys } from "../../api/personal";
import { TextField } from "../../components/AuthCard";
import { useMonthParam } from "../../hooks/useUrlParams";
import { rangeText } from "../cycle";
import { CategorySpend } from "../dashboard/CategorySpend";
import { TotalsCards } from "../dashboard/TotalsCards";
import { describePersonalError } from "../personal/errors";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { hogarLink } from "./links";

/**
 * Resumen of one household user for a pay cycle: her totals and what each Gasto category spent. Same cards as her
 * own Panel, but every link stays inside the Hogar view (it never points at the family budget, which is the owner's
 * own) and carries the month. The month lives in `?mes=`; empty is the current cycle.
 */
export function Resumen({ userId }: { userId: string }) {
  const [picked, setPicked] = useMonthParam();
  const mine = useQuery({
    queryKey: personalKeys.dashboard(picked, userId),
    queryFn: () => getPersonalDashboard(picked, userId),
    retry: false,
  });
  const month = picked || mine.data?.month || "";
  const hint = mine.data ? rangeText(mine.data.period_start, mine.data.period_end) : undefined;
  // Links carry the month the figures are for (the server's own label when none was picked).
  const linkMonth = mine.data?.month ?? picked;

  return (
    <section aria-label="Resumen" className="space-y-4">
      <div className="w-full min-w-0 max-w-full sm:w-56">
        <TextField label="Mes" type="month" hint={hint} value={month} onChange={(e) => e.target.value && setPicked(e.target.value)} />
      </div>
      {mine.isPending && (
        <p role="status" className="flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Cargando el resumen…
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
            compact
            dashboard={mine.data}
            links={{
              income: hogarLink(userId, "ingresos", { mes: linkMonth }),
              expenses: hogarLink(userId, "gastos", { mes: linkMonth }),
              savings: hogarLink(userId, "ahorros", { mes: linkMonth }),
            }}
          />
          <CategorySpend
            categories={mine.data.categories}
            total={mine.data.expenses}
            linkFor={(c) => hogarLink(userId, "gastos", { mes: linkMonth, categoria: c.category_id })}
          />
        </div>
      )}
    </section>
  );
}
