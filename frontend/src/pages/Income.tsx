import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import type { Income as IncomeRow, SaveIncomeResult } from "../api/income";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { IncomeForm } from "./income/IncomeForm";
import { IncomeList } from "./income/IncomeList";
import { SaveSummary } from "./income/SaveSummary";
import { describeIncomeError } from "./income/errors";
import { ErrorBanner, secondaryButton } from "./settings/ui";
import { useCyclePeriod } from "./useCycle";

export function Income() {
  const { month, setMonth, rangeHint } = useCyclePeriod();
  const [editing, setEditing] = useState<IncomeRow | null>(null);
  const [result, setResult] = useState<SaveIncomeResult | null>(null);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        Ingresos
      </h1>
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
            <IncomeForm
              key={editing?.id ?? "new"}
              settings={settings.data}
              editing={editing}
              onSaved={(r) => {
                setResult(r);
                setEditing(null);
              }}
              onCancelEdit={() => setEditing(null)}
            />
            {result && <SaveSummary result={result} />}
          </div>
        )}
      </div>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Ingresos del mes</h2>
          <div className="w-full sm:w-56">
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
              window.scrollTo({ top: 0, behavior: "smooth" });
            }}
            onDeleted={(id) => setEditing((cur) => (cur?.id === id ? null : cur))}
          />
        )}
      </div>
    </section>
  );
}
