import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import type { InvoiceState, InvoiceWarning, Periodicity } from "../api/invoices";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { SelectField } from "./expenses/SelectField";
import { InvoiceDetail } from "./invoices/InvoiceDetail";
import { InvoiceList } from "./invoices/InvoiceList";
import { PrepareForm } from "./invoices/PrepareForm";
import { describeInvoiceError } from "./invoices/errors";
import { STATE_LABEL } from "./invoices/labels";
import { ErrorBanner, secondaryButton } from "./settings/ui";

export function Invoices() {
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const [period, setPeriod] = useState("");
  const [state, setState] = useState<InvoiceState | "">("");
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [periodicity, setPeriodicity] = useState<Periodicity>("mensual");
  const [warnings, setWarnings] = useState<InvoiceWarning[]>([]);
  const detailRef = useRef<HTMLDivElement>(null);

  function select(id: number, w: InvoiceWarning[] = [], p: Periodicity = "mensual") {
    setSelectedId(id);
    setWarnings(w);
    setPeriodicity(p);
    // Let the detail mount before scrolling to it.
    setTimeout(() => detailRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 0);
  }

  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        Facturas
      </h1>
      <p className="mt-1 text-sm text-muted">Prepara el checklist de tus facturas CFDI, da seguimiento a su estado y guarda los archivos emitidos.</p>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {settings.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando configuración…
          </p>
        )}
        {settings.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeInvoiceError(settings.error)}</ErrorBanner>
            <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {settings.data && <PrepareForm settings={settings.data} onPrepared={(detail, p) => select(detail.invoice.id, detail.warnings, p)} />}
      </div>

      {selectedId !== null && settings.data && (
        <div ref={detailRef} className="mt-6 scroll-mt-4 rounded-xl border border-border bg-surface p-4 sm:p-6">
          <InvoiceDetail
            // A new selection resets the detail's own state (periodicity, cancel confirmation).
            key={`${selectedId}-${periodicity}`}
            invoiceId={selectedId}
            clients={settings.data.clients}
            initialPeriodicity={periodicity}
            warnings={warnings}
            onWarnings={setWarnings}
            onClose={() => setSelectedId(null)}
          />
        </div>
      )}

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Historial de facturas</h2>
          <div className="grid w-full gap-3 sm:w-auto sm:grid-cols-[14rem_12rem_auto]">
            <TextField label="Periodo" type="month" value={period} onChange={(e) => setPeriod(e.target.value)} />
            <SelectField label="Estado" value={state} onChange={(e) => setState(e.target.value as InvoiceState | "")}>
              <option value="">Todos</option>
              {(Object.keys(STATE_LABEL) as InvoiceState[]).map((s) => (
                <option key={s} value={s}>
                  {STATE_LABEL[s]}
                </option>
              ))}
            </SelectField>
            {(period || state) && (
              <button
                type="button"
                onClick={() => {
                  setPeriod("");
                  setState("");
                }}
                className={`${secondaryButton} self-end`}
              >
                Quitar filtros
              </button>
            )}
          </div>
        </div>
        <InvoiceList filter={{ period, state }} clients={settings.data?.clients ?? []} selectedId={selectedId} onSelect={(id) => select(id)} />
      </div>
    </section>
  );
}
