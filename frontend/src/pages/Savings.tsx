import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { listSavings, savingsKeys, type Saving } from "../api/savings";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { DeepLinkNote } from "../components/DeepLinkNote";
import { FilterChip } from "../components/FilterChip";
import { InlineFormPanel } from "../components/InlineFormPanel";
import { PortfolioPanel } from "./savings/PortfolioPanel";
import { SavingForm } from "./savings/SavingForm";
import { SavingsList } from "./savings/SavingsList";
import { TransferForm } from "./savings/TransferForm";
import { ValuationForm } from "./savings/ValuationForm";
import { describeSavingsError } from "./savings/errors";
import { ErrorBanner, secondaryButton } from "./settings/ui";
import { useDeepLink } from "../hooks/useDeepLink";
import { useCategoryFilter } from "./useCategoryFilter";
import { useUrlCyclePeriod } from "./useCycle";
import { PageTitle } from "../components/PageTitle";
import { KIND } from "../lib/tones";

const panel = "mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6";

/**
 * The month lives in the URL (`?mes=`), as do the category filter (`?categoria=<name>`, a clearable chip) and a deep
 * link to one saving's edit form (`?editar=<id>`): the Panel's links open the page on them.
 */
export function Savings() {
  const { month, setMonth, rangeHint } = useUrlCyclePeriod();
  const [editing, setEditing] = useState<Saving | null>(null);
  const [saved, setSaved] = useState(false);
  // The create form stays collapsed until asked for; editing a row opens it by itself.
  const [formOpen, setFormOpen] = useState(false);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const category = useCategoryFilter("Ahorro", settings.data?.categories, settings.isPending);
  // Same query as the list below (one request): the deep link needs the loaded rows to find its saving.
  const loaded = useQuery({ queryKey: savingsKeys.list(month ?? ""), queryFn: () => listSavings(month ?? ""), enabled: month !== null, retry: false });
  // A transfer has no edit form (only delete), so a link to one leaves the page as it is.
  const deep = useDeepLink({ param: "editar", items: loaded.data, open: editing, onOpen: setEditing, canOpen: (s) => !s.transfer_id });

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={KIND.Ahorro.icon} tone={KIND.Ahorro.tone}>Ahorros</PageTitle>
      <p className="mt-1 text-sm text-muted">Registra tus aportaciones, traspasos y valuaciones, y revisa tu portafolio.</p>

      {deep.missing && <DeepLinkNote>No se encontró el ahorro del enlace en este periodo. Puedes buscarlo en la lista o cambiar el mes.</DeepLinkNote>}

      <div className={panel}>
        <h2 className="mb-4 text-lg font-semibold tracking-tight">Portafolio</h2>
        <PortfolioPanel />
      </div>

      {settings.isPending && (
        <div className={panel}>
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando configuración…
          </p>
        </div>
      )}
      {settings.isError && (
        <div className={panel}>
          <div className="space-y-3">
            <ErrorBanner>{describeSavingsError(settings.error)}</ErrorBanner>
            <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        </div>
      )}
      {settings.data && (
        <>
          <div className={panel}>
            <InlineFormPanel id="saving-form-panel" label="+ Nuevo ahorro" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={editing !== null}>
              <SavingForm
                key={editing?.id ?? "new"}
                settings={settings.data}
                editing={editing}
                onSaved={() => {
                  setSaved(true);
                  setEditing(null);
                  setFormOpen(false);
                }}
                onCancelEdit={() => {
                  setEditing(null);
                  setFormOpen(false);
                }}
              />
            </InlineFormPanel>
            {saved && (
              <p role="status" className="mt-4 text-sm">
                Ahorro guardado.
              </p>
            )}
          </div>
          <div className={panel}>
            <TransferForm settings={settings.data} />
          </div>
          <div className={panel}>
            <ValuationForm settings={settings.data} />
          </div>
        </>
      )}

      <div className={panel}>
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Ahorros del mes</h2>
          <div className="w-full min-w-0 max-w-full sm:w-56">
            <TextField label="Mes" type="month" hint={rangeHint || undefined} value={month ?? ""} onChange={(e) => e.target.value && setMonth(e.target.value)} />
          </div>
        </div>
        {category.filter && (
          <FilterChip label={`Categoría: ${category.filter}`} clearLabel={`Quitar filtro de categoría ${category.filter}`} onClear={category.clear} />
        )}
        {month === null ? (
          <p role="status" className="text-sm text-muted">
            Cargando periodo…
          </p>
        ) : (
          <SavingsList
            month={month}
            category={category.filter}
            waiting={category.pending}
            instruments={settings.data?.instruments.instruments ?? []}
            editingId={editing?.id ?? null}
            onEdit={(s) => {
              setSaved(false);
              setEditing(s);
            }}
            onDeleted={(id) => setEditing((cur) => (cur?.id === id ? null : cur))}
          />
        )}
      </div>
    </section>
  );
}
