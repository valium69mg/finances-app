import { useRef, useState } from "react";
import { CheckCircle2 } from "lucide-react";
import type { Filing, FilingResult, FilingStatus } from "../api/taxFiling";
import { TextField } from "../components/AuthCard";
import { SelectField } from "./expenses/SelectField";
import { todayISO } from "./expenses/money";
import { secondaryButton } from "./settings/ui";
import { FilingDetail } from "./taxfiling/FilingDetail";
import { FilingHistory } from "./taxfiling/FilingHistory";
import { FILING_STATUS_LABEL, periodLabel } from "./taxfiling/labels";
import { PaymentDialog } from "./taxfiling/PaymentDialog";
import { PendingPeriods } from "./taxfiling/PendingPeriods";
import { UnfiledInvoices } from "./taxfiling/UnfiledInvoices";

/** History of filed declarations with their payment status, the periods still to file and the payment dialog. */
export function FiledRecords() {
  const [year, setYear] = useState("");
  const [status, setStatus] = useState<FilingStatus | "">("");
  const [selected, setSelected] = useState<string | null>(null);
  const [paying, setPaying] = useState<Filing | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const detailRef = useRef<HTMLDivElement>(null);

  // The year field only filters once it is a full four-digit year.
  const yearFilter = /^\d{4}$/.test(year) ? year : "";

  function select(period: string) {
    setSelected(period);
    // Let the detail mount before scrolling to it.
    setTimeout(() => detailRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 0);
  }

  function onPaid(result: FilingResult) {
    setPaying(null);
    setNotice(
      `Pago de ${periodLabel(result.filing.period)} registrado.${result.filing.expense_movement_id !== null ? " Se registró el gasto de Impuestos." : ""}`,
    );
  }

  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        Declaraciones presentadas
      </h1>
      <p className="mt-1 text-sm text-muted">Historial de tus declaraciones RESICO, el estado de su pago y los periodos que aún te faltan por declarar.</p>

      {notice && (
        <p role="status" className="mt-6 flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span className="min-w-0 flex-1 break-words">{notice}</span>
          <button type="button" onClick={() => setNotice(null)} className="focus-ring rounded font-medium underline">
            Cerrar aviso
          </button>
        </p>
      )}

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <h2 className="mb-4 text-lg font-semibold tracking-tight">Periodos pendientes de declarar</h2>
        <PendingPeriods />
      </div>

      <UnfiledInvoices />

      {selected !== null && (
        <div ref={detailRef} className="mt-6 scroll-mt-4 rounded-xl border border-border bg-surface p-4 sm:p-6">
          <FilingDetail
            // A new selection resets the detail's own state (delete confirmation).
            key={selected}
            period={selected}
            onClose={() => setSelected(null)}
            onPay={setPaying}
            onDeleted={(period) => {
              setSelected(null);
              setNotice(`Se eliminó el registro de ${periodLabel(period)}. Sus facturas quedaron sin declarar.`);
            }}
          />
        </div>
      )}

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Historial de declaraciones</h2>
          <div className="grid w-full gap-3 sm:w-auto sm:grid-cols-[10rem_12rem_auto]">
            <TextField label="Año" inputMode="numeric" maxLength={4} placeholder={todayISO().slice(0, 4)} value={year} onChange={(e) => setYear(e.target.value.trim())} autoComplete="off" />
            <SelectField label="Estado del pago" value={status} onChange={(e) => setStatus(e.target.value as FilingStatus | "")}>
              <option value="">Todos</option>
              {(["pendiente", "pagada"] as const).map((s) => (
                <option key={s} value={s}>
                  {FILING_STATUS_LABEL[s]}
                </option>
              ))}
            </SelectField>
            {(year || status) && (
              <button
                type="button"
                onClick={() => {
                  setYear("");
                  setStatus("");
                }}
                className={`${secondaryButton} self-end`}
              >
                Quitar filtros
              </button>
            )}
          </div>
        </div>
        <FilingHistory filter={{ year: yearFilter, status }} selectedPeriod={selected} onSelect={select} onPay={setPaying} />
      </div>

      {paying && <PaymentDialog key={paying.period} filing={paying} onClose={() => setPaying(null)} onPaid={onPaid} />}
    </section>
  );
}
