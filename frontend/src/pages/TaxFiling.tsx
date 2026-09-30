import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Loader2, Calculator } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { previewTaxFiling, taxFilingKeys, type FilingResult, type TaxPreview } from "../api/taxFiling";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { todayISO } from "./expenses/money";
import { ErrorBanner, primaryButton, secondaryButton } from "./settings/ui";
import { Breakdown } from "./taxfiling/Breakdown";
import { describeTaxFilingError } from "./taxfiling/errors";
import { InvoiceRefs, TaxWarnings } from "./taxfiling/InvoiceRefs";
import { dateLabel, periodLabel, previousPeriod } from "./taxfiling/labels";
import { isPaidAmount } from "./taxfiling/payment";
import { RegisterForm } from "./taxfiling/RegisterForm";
import { FilingStatusBadge } from "./taxfiling/StatusBadge";

const isPeriod = (v: string) => /^\d{4}-(0[1-9]|1[0-2])$/.test(v);

interface Applied {
  period: string;
  iva: string;
}

function PreviewBody({ preview, onRegistered }: { preview: TaxPreview; onRegistered: (r: FilingResult) => void }) {
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const clients = settings.data?.clients ?? [];
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-tight">Declaración de {periodLabel(preview.period)}</h2>
        {preview.filing && <FilingStatusBadge status={preview.filing.status} />}
      </div>

      <TaxWarnings warnings={preview.warnings} />

      <Breakdown amounts={preview} />

      <section aria-labelledby="included-title" className="space-y-2">
        <h3 id="included-title" className="text-base font-semibold tracking-tight">
          Facturas incluidas
        </h3>
        <p className="text-sm text-muted">Solo cuentan las facturas emitidas del periodo; las preparadas y las canceladas no.</p>
        <InvoiceRefs invoices={preview.invoices} clients={clients} />
      </section>

      {preview.filing ? (
        <div className="rounded-lg border border-border p-4 text-sm">
          <p>
            Este periodo se presentó el <span className="font-medium">{dateLabel(preview.filing.filing_date)}</span>
            {preview.filing.folio && <> (folio {preview.filing.folio})</>}.
          </p>
          <Link to="/declaraciones-presentadas" className="mt-2 inline-block font-medium underline">
            Ver en Declaraciones presentadas
          </Link>
        </div>
      ) : (
        // A new calculation (period or creditable IVA) starts a fresh form with matching amounts.
        <RegisterForm key={`${preview.period}-${preview.iva_acreditable}`} preview={preview} onRegistered={onRegistered} />
      )}
    </div>
  );
}

/** Computes the monthly RESICO declaration of a period and registers the filing made in the SAT portal. */
export function TaxFiling() {
  const [params] = useSearchParams();
  const fromLink = params.get("period") ?? "";
  const initial = isPeriod(fromLink) ? fromLink : previousPeriod(todayISO().slice(0, 7));

  const [period, setPeriod] = useState(initial);
  const [iva, setIva] = useState("");
  const [errors, setErrors] = useState<{ period?: string; iva?: string }>({});
  const [applied, setApplied] = useState<Applied>({ period: initial, iva: "" });
  const [registered, setRegistered] = useState<FilingResult | null>(null);

  const preview = useQuery({
    queryKey: taxFilingKeys.preview(applied.period, applied.iva),
    queryFn: () => previewTaxFiling(applied.period, applied.iva),
    retry: false,
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: typeof errors = {};
    if (!isPeriod(period)) found.period = "Elige el periodo (mes y año) que vas a declarar.";
    if (iva.trim() !== "" && !isPaidAmount(iva)) found.iva = "Escribe un monto de cero o más, con máximo 2 decimales, por ejemplo 800.50.";
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    setRegistered(null);
    setApplied({ period, iva: iva.trim() });
  }

  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        Declaración
      </h1>
      <p className="mt-1 text-sm text-muted">
        Calcula tu declaración mensual RESICO con las facturas emitidas del periodo y regístrala cuando la presentes en el portal del SAT.
      </p>

      <form noValidate onSubmit={onSubmit} aria-label="Calcular declaración" className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="grid gap-4 sm:grid-cols-[14rem_1fr_auto] sm:items-start">
          <TextField label="Periodo" type="month" value={period} onChange={(e) => setPeriod(e.target.value)} error={errors.period} />
          <TextField
            label="IVA acreditable (opcional)"
            hint="IVA de tus gastos deducibles del periodo, en pesos."
            inputMode="decimal"
            value={iva}
            onChange={(e) => setIva(e.target.value)}
            error={errors.iva}
            autoComplete="off"
          />
          <button type="submit" disabled={preview.isFetching} aria-busy={preview.isFetching} className={`${primaryButton} sm:mt-7`}>
            {preview.isFetching ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Calculator className="h-4 w-4" aria-hidden="true" />}
            Calcular
          </button>
        </div>
      </form>

      {registered && (
        <p role="status" className="mt-6 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span>
            Declaración de {periodLabel(registered.filing.period)} registrada
            {registered.filing.status === "pagada" ? " y pagada" : " con el pago pendiente"}.
            {registered.filing.expense_movement_id !== null && " Se registró el gasto de Impuestos."}
          </span>
          <Link to="/declaraciones-presentadas" className="font-medium underline">
            Ver declaraciones presentadas
          </Link>
        </p>
      )}

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {preview.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Calculando declaración…
          </p>
        )}
        {preview.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeTaxFilingError(preview.error)}</ErrorBanner>
            <div className="flex flex-wrap items-center gap-3">
              <button type="button" onClick={() => void preview.refetch()} className={secondaryButton}>
                Reintentar
              </button>
              {preview.error instanceof ApiError && preview.error.code === "settings_incomplete" && (
                <Link to="/configuracion" className="text-sm font-medium underline">
                  Ir a Configuración
                </Link>
              )}
            </div>
          </div>
        )}
        {preview.data && <PreviewBody preview={preview.data} onRegistered={setRegistered} />}
      </div>
    </section>
  );
}
