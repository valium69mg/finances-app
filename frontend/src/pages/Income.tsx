import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { incomeKeys, listIncome, type Income as IncomeRow, type SaveIncomeResult } from "../api/income";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { DeepLinkNote } from "../components/DeepLinkNote";
import { InlineFormPanel } from "../components/InlineFormPanel";
import { IncomeDialog } from "./income/IncomeDialog";
import { IncomeForm } from "./income/IncomeForm";
import { IncomeList } from "./income/IncomeList";
import { SaveSummary } from "./income/SaveSummary";
import { describeIncomeError } from "./income/errors";
import { ErrorBanner, secondaryButton } from "./settings/ui";
import { useDeepLink } from "../hooks/useDeepLink";
import { useUrlCyclePeriod } from "./useCycle";
import { PageTitle } from "../components/PageTitle";
import { KIND } from "../lib/tones";

/** The month lives in the URL (`?mes=`) and `?editar=<id>` opens one income's edit dialog: the Panel's links use both. */
export function Income() {
  const { month, setMonth, rangeHint } = useUrlCyclePeriod();
  const [editing, setEditing] = useState<IncomeRow | null>(null);
  // The create form stays collapsed until asked for; editing a row opens its dialog.
  const [formOpen, setFormOpen] = useState(false);
  const [result, setResult] = useState<SaveIncomeResult | null>(null);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  // Same query as the list below (one request): the deep link needs the loaded rows to find its income.
  const loaded = useQuery({ queryKey: incomeKeys.list(month ?? ""), queryFn: () => listIncome(month ?? ""), enabled: month !== null, retry: false });
  const deep = useDeepLink({ param: "editar", items: loaded.data, open: editing, onOpen: setEditing });

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={KIND.Ingreso.icon} tone={KIND.Ingreso.tone}>Ingresos</PageTitle>
      <p className="mt-1 text-sm text-muted">Registra tus ingresos y revisa tu total del mes y el ISR RESICO estimado.</p>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {settings.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando configuración…
          </p>
        )}
        {settings.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeIncomeError(settings.error)}</ErrorBanner>
            <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {settings.data && (
          <div className="space-y-4">
            <InlineFormPanel id="income-form-panel" label="+ Nuevo ingreso" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={false}>
              <IncomeForm
                settings={settings.data}
                editing={null}
                onSaved={(r) => {
                  setResult(r);
                  setFormOpen(false);
                }}
                onCancelEdit={() => setFormOpen(false)}
              />
            </InlineFormPanel>
            {result && <SaveSummary result={result} />}
          </div>
        )}
      </div>

      {deep.missing && <DeepLinkNote>No se encontró el ingreso del enlace en este periodo. Puedes buscarlo en la lista o cambiar el mes.</DeepLinkNote>}

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Ingresos del mes</h2>
          <div className="w-full min-w-0 max-w-full sm:w-56">
            <TextField label="Mes" type="month" hint={rangeHint || undefined} value={month ?? ""} onChange={(e) => e.target.value && setMonth(e.target.value)} />
          </div>
        </div>
        {month === null ? (
          <p role="status" className="text-sm text-muted">
            Cargando periodo…
          </p>
        ) : (
          <IncomeList
            month={month}
            editingId={editing?.id ?? null}
            onEdit={(i) => {
              setResult(null);
              setEditing(i);
            }}
          />
        )}
      </div>

      {editing && settings.data && (
        <IncomeDialog
          settings={settings.data}
          income={editing}
          onClose={() => setEditing(null)}
          onSaved={(r) => {
            setResult(r);
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}
