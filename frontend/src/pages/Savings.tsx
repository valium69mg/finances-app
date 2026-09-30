import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import type { Saving } from "../api/savings";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { PortfolioPanel } from "./savings/PortfolioPanel";
import { SavingForm } from "./savings/SavingForm";
import { SavingsList } from "./savings/SavingsList";
import { TransferForm } from "./savings/TransferForm";
import { ValuationForm } from "./savings/ValuationForm";
import { describeSavingsError } from "./savings/errors";
import { ErrorBanner, secondaryButton } from "./settings/ui";
import { useCyclePeriod } from "./useCycle";

const panel = "mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6";

export function Savings() {
  const { month, setMonth, rangeHint } = useCyclePeriod();
  const [editing, setEditing] = useState<Saving | null>(null);
  const [saved, setSaved] = useState(false);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        Ahorros
      </h1>
      <p className="mt-1 text-sm text-muted">Registra tus aportaciones, traspasos y valuaciones, y revisa tu portafolio.</p>

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
            <SavingForm
              key={editing?.id ?? "new"}
              settings={settings.data}
              editing={editing}
              onSaved={() => {
                setSaved(true);
                setEditing(null);
              }}
              onCancelEdit={() => setEditing(null)}
            />
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
        {month === null ? (
          <p role="status" className="text-sm text-muted">
            Cargando periodo…
          </p>
        ) : (
          <SavingsList
            month={month}
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
