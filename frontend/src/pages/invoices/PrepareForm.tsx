import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { invoiceKeys, prepareInvoice, type InvoiceDetail, type Periodicity, type PrepareInput } from "../../api/invoices";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { formatMoney, formatRatePercent, isPositiveDecimal, todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, fieldGrid, primaryButton } from "../settings/ui";
import { ClientSelect } from "./ClientSelect";
import { describeInvoiceError } from "./errors";
import { PERIODICITY_LABEL, isUsaClient } from "./labels";

interface Props {
  settings: AllSettings;
  /** Called with the prepared invoice and the periodicity used for its checklist. */
  onPrepared: (detail: InvoiceDetail, periodicity: Periodicity) => void;
}

type Errors = { client?: string; date?: string; subtotal?: string; rate?: string; amount?: string };

/**
 * Prepares an invoice for the SAT portal. Client USA (export of services) takes a
 * subtotal in USD and an exchange rate; every other client takes the total
 * received with IVA included, as in fin.py.
 */
export function PrepareForm({ settings, onPrepared }: Props) {
  const qc = useQueryClient();
  const clients = settings.clients;
  const [clientId, setClientId] = useState(clients.length === 1 ? clients[0].id : "");
  const [date, setDate] = useState(todayISO());
  const [subtotal, setSubtotal] = useState("");
  const [rate, setRate] = useState("");
  const [amount, setAmount] = useState("");
  const [periodicity, setPeriodicity] = useState<Periodicity>("mensual");
  const [errors, setErrors] = useState<Errors>({});
  const clientRef = useRef<HTMLDivElement>(null);
  const amountRef = useRef<HTMLInputElement>(null);
  const subtotalRef = useRef<HTMLInputElement>(null);

  const usa = isUsaClient(clientId);
  const client = clients.find((c) => c.id === clientId);

  const prepare = useMutation({
    mutationFn: (input: PrepareInput) => prepareInvoice(input),
    onSuccess: (detail, input) => {
      void qc.invalidateQueries({ queryKey: invoiceKeys.all });
      onPrepared(detail, input.periodicity ?? "mensual");
      setSubtotal("");
      setRate("");
      setAmount("");
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: Errors = {};
    if (!clientId) found.client = "Elige un cliente.";
    if (!date) found.date = "Elige la fecha de cobro.";
    if (clientId && usa) {
      if (subtotal.trim() !== "" && !isPositiveDecimal(subtotal)) found.subtotal = "Escribe un subtotal mayor a cero, por ejemplo 3500.";
      if (rate.trim() !== "" && !isPositiveDecimal(rate)) found.rate = "Escribe un tipo de cambio válido, por ejemplo 17.50.";
    } else if (clientId && !isPositiveDecimal(amount)) {
      found.amount = "Escribe el total recibido (con IVA incluido), mayor a cero, por ejemplo 35000.";
    }
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.client) clientRef.current?.querySelector("select")?.focus();
      else if (found.subtotal) subtotalRef.current?.focus();
      else if (found.amount) amountRef.current?.focus();
      return;
    }
    const input: PrepareInput = { client_id: clientId, date };
    if (usa) {
      if (subtotal.trim()) input.subtotal = subtotal.trim();
      if (rate.trim()) input.exchange_rate = rate.trim();
    } else {
      input.amount = amount.trim();
      input.periodicity = periodicity;
    }
    prepare.mutate(input);
  }

  if (clients.length === 0) {
    return (
      <div className="space-y-2">
        <h2 className="text-lg font-semibold tracking-tight">Preparar factura</h2>
        <p className="text-sm text-muted">Aún no hay clientes. Agrégalos en Configuración para preparar facturas.</p>
      </div>
    );
  }

  const salary = settings.general.salary_usd;
  const fx = settings.general.fx_rate_applied;

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="prepare-form-title" className="space-y-5">
      <div>
        <h2 id="prepare-form-title" className="text-lg font-semibold tracking-tight">
          Preparar factura
        </h2>
        <p className="mt-1 text-sm text-muted">
          La app no timbra: prepara el checklist para el portal del SAT, y después marcas la factura como emitida con el XML del CFDI.
        </p>
      </div>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <div ref={clientRef}>
          <ClientSelect label="Cliente" value={clientId} onChange={setClientId} clients={clients} emptyLabel="Elige un cliente" />
          {errors.client && (
            <p role="alert" className="mt-1.5 text-sm text-destructive">
              {errors.client}
            </p>
          )}
        </div>
        <TextField label="Fecha de cobro" type="date" value={date} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        {clientId && usa && (
          <>
            <TextField
              label="Subtotal (USD)"
              hint={`Si lo dejas vacío se usa el sueldo configurado (${formatMoney(salary)} USD).`}
              inputMode="decimal"
              inputRef={subtotalRef}
              value={subtotal}
              onChange={(e) => setSubtotal(e.target.value)}
              error={errors.subtotal}
              autoComplete="off"
            />
            <TextField
              label="Tipo de cambio (opcional)"
              hint={`Si lo dejas vacío se usa el de configuración (${fx}).`}
              inputMode="decimal"
              value={rate}
              onChange={(e) => setRate(e.target.value)}
              error={errors.rate}
              autoComplete="off"
            />
          </>
        )}
        {clientId && !usa && (
          <>
            <TextField
              label="Total recibido (IVA incluido)"
              hint={client ? `El IVA (${formatRatePercent(client.iva_rate)}) se calcula sobre este monto.` : undefined}
              inputMode="decimal"
              inputRef={amountRef}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              error={errors.amount}
              autoComplete="off"
            />
            <SelectField label="Periodicidad (factura global)" value={periodicity} onChange={(e) => setPeriodicity(e.target.value as Periodicity)}>
              {(Object.keys(PERIODICITY_LABEL) as Periodicity[]).map((p) => (
                <option key={p} value={p}>
                  {PERIODICITY_LABEL[p]}
                </option>
              ))}
            </SelectField>
          </>
        )}
      </div>

      {prepare.isError && <ErrorBanner>{describeInvoiceError(prepare.error)}</ErrorBanner>}

      <button type="submit" disabled={prepare.isPending} aria-busy={prepare.isPending} className={primaryButton}>
        {prepare.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Plus className="h-4 w-4" aria-hidden="true" />}
        {prepare.isPending ? "Preparando…" : "Preparar factura"}
      </button>
    </form>
  );
}
