import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowRightLeft, CheckCircle2, LineChart, Loader2, Plus } from "lucide-react";
import { listSavings, savingsKeys, type Saving } from "../api/savings";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { DeepLinkNote } from "../components/DeepLinkNote";
import { FilterChip } from "../components/FilterChip";
import { Modal } from "../components/Modal";
import { PortfolioPanel } from "./savings/PortfolioPanel";
import { SavingDialog, TransferLegDialog } from "./savings/SavingDialogs";
import { SavingsList } from "./savings/SavingsList";
import { TransferForm } from "./savings/TransferForm";
import { ValuationForm } from "./savings/ValuationForm";
import { describeSavingsError } from "./savings/errors";
import { ErrorBanner, primaryButton, secondaryButton } from "./settings/ui";
import { useDeepLink } from "../hooks/useDeepLink";
import { useCategoryFilter } from "./useCategoryFilter";
import { useUrlCyclePeriod } from "./useCycle";
import { PageTitle } from "../components/PageTitle";
import { KIND } from "../lib/tones";

const panel = "mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6";

type Dialog = { kind: "saving"; saving: Saving | null } | { kind: "leg"; saving: Saving } | { kind: "transfer" } | { kind: "valuation" } | null;

/**
 * Ahorros, summary first: the portfolio on top, "Nuevo ahorro", "Transferir" and "Actualizar valuación" in the header
 * (each opens its form in a dialog) and the month's savings below, where a row opens its edit dialog in place.
 * The month lives in the URL (`?mes=`), as do the category filter (`?categoria=<name>`, a clearable chip) and a deep
 * link to one saving's edit dialog (`?editar=<id>`): the Panel's links open the page on them.
 */
export function Savings() {
  const { month, setMonth, rangeHint } = useUrlCyclePeriod();
  const [dialog, setDialog] = useState<Dialog>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const category = useCategoryFilter("Ahorro", settings.data?.categories, settings.isPending);
  // Same query as the list below (one request): the deep link needs the loaded rows to find its saving.
  const loaded = useQuery({ queryKey: savingsKeys.list(month ?? ""), queryFn: () => listSavings(month ?? ""), enabled: month !== null, retry: false });
  const editing = dialog?.kind === "saving" ? dialog.saving : null;
  // A transfer has no edit form (only delete), so a link to one leaves the page as it is.
  const deep = useDeepLink({ param: "editar", items: loaded.data, open: editing, onOpen: (s) => setDialog({ kind: "saving", saving: s }), canOpen: (s) => !s.transfer_id });

  const open = (next: Dialog) => {
    setNotice(null);
    setDialog(next);
  };
  const done = (message: string) => {
    setDialog(null);
    setNotice(message);
  };

  const actions = settings.data ? (
    <>
      <button type="button" onClick={() => open({ kind: "saving", saving: null })} className={primaryButton}>
        <Plus className="h-4 w-4" aria-hidden="true" />
        Nuevo ahorro
      </button>
      <button type="button" onClick={() => open({ kind: "transfer" })} className={secondaryButton}>
        <ArrowRightLeft className="h-4 w-4" aria-hidden="true" />
        Transferir
      </button>
      <button type="button" onClick={() => open({ kind: "valuation" })} className={secondaryButton}>
        <LineChart className="h-4 w-4" aria-hidden="true" />
        Actualizar valuación
      </button>
    </>
  ) : undefined;

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={KIND.Ahorro.icon} tone={KIND.Ahorro.tone} action={actions}>
        Ahorros
      </PageTitle>
      <p className="mt-1 text-sm text-muted">Registra tus aportaciones, traspasos y valuaciones, y revisa tu portafolio.</p>

      {deep.missing && <DeepLinkNote>No se encontró el ahorro del enlace en este periodo. Puedes buscarlo en la lista o cambiar el mes.</DeepLinkNote>}

      {notice && (
        <p role="status" className="mt-6 flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span className="min-w-0 break-words">{notice}</span>
        </p>
      )}

      <section aria-labelledby="portfolio-title" className="mt-6 space-y-4">
        <h2 id="portfolio-title" className="text-lg font-semibold tracking-tight">
          Portafolio
        </h2>
        <PortfolioPanel />
      </section>

      {settings.isPending && (
        <p role="status" className="mt-6 flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Cargando configuración…
        </p>
      )}
      {settings.isError && (
        <div className="mt-6 space-y-3">
          <ErrorBanner>{describeSavingsError(settings.error)}</ErrorBanner>
          <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
            Reintentar
          </button>
        </div>
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
            onOpen={(s) => open(s.transfer_id ? { kind: "leg", saving: s } : { kind: "saving", saving: s })}
          />
        )}
      </div>

      {settings.data && dialog?.kind === "saving" && (
        <SavingDialog
          settings={settings.data}
          saving={dialog.saving}
          onClose={() => setDialog(null)}
          onSaved={() => done("Ahorro guardado.")}
          onDeleted={() => done("Ahorro eliminado.")}
        />
      )}
      {dialog?.kind === "leg" && <TransferLegDialog saving={dialog.saving} onClose={() => setDialog(null)} onDeleted={() => done("Traspaso eliminado.")} />}
      {settings.data && dialog?.kind === "transfer" && (
        <Modal title="Traspasar entre instrumentos" onClose={() => setDialog(null)}>
          <TransferForm settings={settings.data} onDone={() => done("Traspaso registrado.")} onCancel={() => setDialog(null)} />
        </Modal>
      )}
      {settings.data && dialog?.kind === "valuation" && (
        <Modal title="Registrar valuación" onClose={() => setDialog(null)}>
          <ValuationForm settings={settings.data} onDone={() => done("Valuación registrada.")} onCancel={() => setDialog(null)} />
        </Modal>
      )}
    </section>
  );
}
