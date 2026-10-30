import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ListPlus, Loader2, Plus } from "lucide-react";
import { getPersonalPeriod, listPersonalCategories, personalKeys, type PersonalMovement } from "../../api/personal";
import { TextField } from "../../components/AuthCard";
import { FilterChip } from "../../components/FilterChip";
import { PageTitle } from "../../components/PageTitle";
import { useCategoryParam, useMonthParam } from "../../hooks/useUrlParams";
import { KIND } from "../../lib/tones";
import { rangeText } from "../cycle";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { CategoriesDialog } from "./CategoriesDialog";
import type { PersonalPageConfig } from "./config";
import { describePersonalError } from "./errors";
import { MovementDialog } from "./MovementDialog";
import { MovementList } from "./MovementList";
import { usePersonalScope } from "./scope";

/**
 * Page shared by Mis ingresos, Mis gastos and Mis ahorros: "Nuevo …" (and "Administrar categorías") in the header, period
 * picker, total, a list of rows that open the edit dialog (which also deletes), and a dialog to create.
 * The household role cannot read the settings, so the period is the YYYY-MM label she picks and the server answers
 * for the current cycle until she does. The month lives in the URL (`?mes=`, the Panel's links open on it) and the
 * range hint comes from /personal/period. `?categoria=<id>` filters the list by one of the page's categories and shows
 * as a clearable chip; an id that is not one of them is ignored.
 *
 * Inside the owner's "Hogar" view (see PersonalScope) the same page is read-only: every read carries the household
 * user's id and nothing that writes is rendered (no new, edit, delete or category management; the rows are plain), and the Hogar page owns
 * the heading.
 */
export function PersonalMovements({ config, manageCategories = false }: { config: PersonalPageConfig; manageCategories?: boolean }) {
  const { icon, tone } = KIND[config.kind];
  const { userId, readOnly } = usePersonalScope();
  const [month, setMonth] = useMonthParam();
  const [rawCategory, setCategory] = useCategoryParam();
  const [dialog, setDialog] = useState<{ editing: PersonalMovement | null } | null>(null);
  const [categoriesOpen, setCategoriesOpen] = useState(false);
  const categories = useQuery({
    queryKey: personalKeys.categories(config.kind, userId ?? undefined),
    queryFn: () => listPersonalCategories(config.kind, userId ?? undefined),
    retry: false,
  });

  const period = useQuery({
    queryKey: personalKeys.period(month, userId ?? undefined),
    queryFn: () => getPersonalPeriod(month, userId ?? undefined),
    retry: false,
  });
  const hint = period.data ? rangeText(period.data.period_start, period.data.period_end) : undefined;
  const filter = categories.data?.find((c) => c.id === rawCategory);

  const actions =
    !readOnly && categories.data ? (
      <>
        <button type="button" onClick={() => setDialog({ editing: null })} className={primaryButton}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          {config.newLabel}
        </button>
        {manageCategories && (
          <button type="button" onClick={() => setCategoriesOpen(true)} className={secondaryButton}>
            <ListPlus className="h-4 w-4" aria-hidden="true" />
            Administrar categorías
          </button>
        )}
      </>
    ) : undefined;

  return (
    <section aria-labelledby={readOnly ? undefined : "page-title"} aria-label={readOnly ? config.listTitle : undefined}>
      {!readOnly && (
        <>
          <PageTitle icon={icon} tone={tone} action={actions}>
            {config.title}
          </PageTitle>
          <p className="mt-1 text-sm text-muted">{config.intro}</p>
        </>
      )}

      <div className={`${readOnly ? "" : "mt-6 "}rounded-xl border border-border bg-surface p-4 sm:p-6`}>
        {categories.isPending && (
          <p role="status" className="mb-4 flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando categorías…
          </p>
        )}
        {categories.isError && (
          <div className="mb-4 space-y-3">
            <ErrorBanner>{describePersonalError(categories.error)}</ErrorBanner>
            <button type="button" onClick={() => void categories.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">{config.listTitle}</h2>
          <div className="w-full min-w-0 max-w-full sm:w-56">
            <TextField
              label="Mes"
              type="month"
              hint={hint}
              value={month || period.data?.month || ""}
              onChange={(e) => setMonth(e.target.value)}
            />
          </div>
        </div>
        {filter && <FilterChip label={`Categoría: ${filter.name}`} clearLabel={`Quitar filtro de categoría ${filter.name}`} onClear={() => setCategory(0)} />}
        <MovementList
          config={config}
          month={month}
          category={filter}
          ready={!categories.isPending}
          onEdit={(m) => setDialog({ editing: m })}
        />
      </div>

      {!readOnly && dialog && categories.data && (
        <MovementDialog
          key={dialog.editing?.id ?? "new"}
          config={config}
          categories={categories.data}
          editing={dialog.editing}
          onClose={() => setDialog(null)}
        />
      )}
      {!readOnly && categoriesOpen && categories.data && <CategoriesDialog categories={categories.data} onClose={() => setCategoriesOpen(false)} />}
    </section>
  );
}
