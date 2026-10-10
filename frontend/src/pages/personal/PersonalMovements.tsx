import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ListPlus, Loader2, Plus } from "lucide-react";
import { listPersonalCategories, personalKeys, type PersonalMovement } from "../../api/personal";
import { TextField } from "../../components/AuthCard";
import { PageTitle } from "../../components/PageTitle";
import { KIND } from "../../lib/tones";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { CategoriesDialog } from "./CategoriesDialog";
import type { PersonalPageConfig } from "./config";
import { describePersonalError } from "./errors";
import { MovementDialog } from "./MovementDialog";
import { MovementList } from "./MovementList";

/**
 * Page shared by Mis ingresos, Mis gastos and Mis ahorros: period picker, total, list, and a dialog to create or edit.
 * The household role cannot read the settings, so the period is the YYYY-MM label the user picks; until she picks one the
 * server answers for the current cycle.
 */
export function PersonalMovements({ config, manageCategories = false }: { config: PersonalPageConfig; manageCategories?: boolean }) {
  const { icon, tone } = KIND[config.kind];
  const [month, setMonth] = useState("");
  const [dialog, setDialog] = useState<{ editing: PersonalMovement | null } | null>(null);
  const [categoriesOpen, setCategoriesOpen] = useState(false);
  const categories = useQuery({
    queryKey: personalKeys.categories(config.kind),
    queryFn: () => listPersonalCategories(config.kind),
    retry: false,
  });

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={icon} tone={tone}>
        {config.title}
      </PageTitle>
      <p className="mt-1 text-sm text-muted">{config.intro}</p>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {categories.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando categorías…
          </p>
        )}
        {categories.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describePersonalError(categories.error)}</ErrorBanner>
            <button type="button" onClick={() => void categories.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {categories.data && (
          <div className="flex flex-wrap gap-3">
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
          </div>
        )}
      </div>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">{config.listTitle}</h2>
          <div className="w-full min-w-0 max-w-full sm:w-56">
            <TextField
              label="Mes"
              type="month"
              hint={month ? undefined : "Mostrando el periodo actual."}
              value={month}
              onChange={(e) => setMonth(e.target.value)}
            />
          </div>
        </div>
        <MovementList config={config} month={month} onEdit={(m) => setDialog({ editing: m })} onDeleted={(id) => setDialog((cur) => (cur?.editing?.id === id ? null : cur))} />
      </div>

      {dialog && categories.data && (
        <MovementDialog
          key={dialog.editing?.id ?? "new"}
          config={config}
          categories={categories.data}
          editing={dialog.editing}
          onClose={() => setDialog(null)}
        />
      )}
      {categoriesOpen && categories.data && <CategoriesDialog categories={categories.data} onClose={() => setCategoriesOpen(false)} />}
    </section>
  );
}
