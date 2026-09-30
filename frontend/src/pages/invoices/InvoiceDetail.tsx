import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, Loader2, X } from "lucide-react";
import { cancelInvoice, getInvoice, invoiceKeys, type Invoice, type InvoiceDetail as Detail, type InvoiceWarning, type Periodicity } from "../../api/invoices";
import type { Client } from "../../api/settings";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { ChecklistView } from "./ChecklistView";
import { DocumentsPanel } from "./DocumentsPanel";
import { describeInvoiceError } from "./errors";
import { IssuePanel } from "./IssuePanel";
import { PERIODICITY_LABEL, clientName, isUsaClient, money } from "./labels";
import { StateBadge } from "./StateBadge";
import { WarningsList } from "./WarningsList";

interface Props {
  invoiceId: number;
  clients: Client[];
  /** Periodicity used when the invoice was just prepared, so its checklist matches. */
  initialPeriodicity: Periodicity;
  /** Warnings of the last operation on this invoice (prepare, issue, XML upload). */
  warnings: InvoiceWarning[];
  onWarnings: (warnings: InvoiceWarning[]) => void;
  onClose: () => void;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className="break-words font-medium tabular-nums">{children}</dd>
    </div>
  );
}

function Summary({ inv, clients }: { inv: Invoice; clients: Client[] }) {
  return (
    <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
      <Field label="Cliente">{clientName(clients, inv.client_id)}</Field>
      <Field label="Fecha de cobro">{inv.collection_date}</Field>
      <Field label="Periodo fiscal">{inv.period}</Field>
      <Field label="Moneda">{inv.exchange_rate ? `${inv.currency} · tipo de cambio ${inv.exchange_rate}` : inv.currency}</Field>
      <Field label="Subtotal">{money(inv.subtotal, inv.currency)}</Field>
      <Field label="IVA">{money(inv.iva, "MXN")}</Field>
      <Field label="Total">{money(inv.total, inv.currency)}</Field>
      <Field label="Depósito esperado">{money(inv.expected_deposit_mxn, "MXN")}</Field>
      <Field label="UUID">{inv.uuid ? <span className="break-all">{inv.uuid}</span> : "Aún sin UUID"}</Field>
      {inv.declaration_period && <Field label="Incluida en la declaración de">{inv.declaration_period}</Field>}
    </dl>
  );
}

export function InvoiceDetail({ invoiceId, clients, initialPeriodicity, warnings, onWarnings, onClose }: Props) {
  const qc = useQueryClient();
  const [periodicity, setPeriodicity] = useState<Periodicity>(initialPeriodicity);
  const [confirmingCancel, setConfirmingCancel] = useState(false);

  const detail = useQuery({
    queryKey: invoiceKeys.detail(invoiceId, periodicity),
    queryFn: () => getInvoice(invoiceId, periodicity),
    retry: false,
  });
  const cancel = useMutation({
    mutationFn: () => cancelInvoice(invoiceId),
    onSuccess: () => {
      setConfirmingCancel(false);
      onWarnings([]);
      return qc.invalidateQueries({ queryKey: invoiceKeys.all });
    },
  });

  const title = `Factura #${invoiceId}`;

  return (
    <section aria-label={`Detalle de la ${title}`} className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="flex flex-wrap items-center gap-2 text-lg font-semibold tracking-tight">
          {title}
          {detail.data && <StateBadge state={detail.data.invoice.state} />}
        </h2>
        <button type="button" onClick={onClose} className={secondaryButton}>
          <X className="h-4 w-4" aria-hidden="true" />
          Cerrar detalle
        </button>
      </div>

      {detail.isPending && (
        <p role="status" className="flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Cargando factura…
        </p>
      )}
      {detail.isError && (
        <div className="space-y-3">
          <ErrorBanner>{describeInvoiceError(detail.error)}</ErrorBanner>
          <button type="button" onClick={() => void detail.refetch()} className={secondaryButton}>
            Reintentar
          </button>
        </div>
      )}
      {detail.data && (
        <DetailBody
          data={detail.data}
          clients={clients}
          periodicity={periodicity}
          onPeriodicity={setPeriodicity}
          warnings={warnings}
          onWarnings={onWarnings}
          cancel={{
            confirming: confirmingCancel,
            pending: cancel.isPending,
            error: cancel.isError ? describeInvoiceError(cancel.error) : null,
            ask: () => {
              cancel.reset();
              setConfirmingCancel(true);
            },
            dismiss: () => setConfirmingCancel(false),
            confirm: () => cancel.mutate(),
          }}
        />
      )}
    </section>
  );
}

interface CancelState {
  confirming: boolean;
  pending: boolean;
  error: string | null;
  ask: () => void;
  dismiss: () => void;
  confirm: () => void;
}

interface BodyProps {
  data: Detail;
  clients: Client[];
  periodicity: Periodicity;
  onPeriodicity: (p: Periodicity) => void;
  warnings: InvoiceWarning[];
  onWarnings: (w: InvoiceWarning[]) => void;
  cancel: CancelState;
}

function DetailBody({ data, clients, periodicity, onPeriodicity, warnings, onWarnings, cancel }: BodyProps) {
  const inv = data.invoice;
  const cancellable = inv.state !== "cancelada";
  return (
    <div className="space-y-5">
      <Summary inv={inv} clients={clients} />
      <WarningsList warnings={warnings} currency={inv.currency} />

      {inv.state === "cancelada" && (
        <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          Esta factura está cancelada: ya no puede emitirse ni cuenta para la declaración. Sus archivos siguen disponibles.
        </p>
      )}

      {inv.state === "preparada" && (
        <IssuePanel
          invoiceId={inv.id}
          onIssued={(res) => {
            onWarnings(res.warnings);
          }}
        />
      )}

      {inv.state !== "preparada" && (
        <DocumentsPanel invoiceId={inv.id} documents={data.documents} canAttach={inv.state === "emitida"} onWarnings={onWarnings} />
      )}

      {!isUsaClient(inv.client_id) && (
        <div className="max-w-xs">
          <SelectField label="Periodicidad (factura global)" value={periodicity} onChange={(e) => onPeriodicity(e.target.value as Periodicity)}>
            {(Object.keys(PERIODICITY_LABEL) as Periodicity[]).map((p) => (
              <option key={p} value={p}>
                {PERIODICITY_LABEL[p]}
              </option>
            ))}
          </SelectField>
        </div>
      )}

      <ChecklistView checklist={data.checklist} />

      {cancellable && (
        <div className="space-y-3 border-t border-border pt-5">
          {cancel.error && <ErrorBanner>{cancel.error}</ErrorBanner>}
          {cancel.confirming ? (
            <div role="group" aria-label={`Confirmar cancelación de la factura #${inv.id}`} className="space-y-3">
              <p className="text-sm">
                ¿Cancelar esta factura? Quedará como cancelada y ya no podrás emitirla ni contará para la declaración.
                {inv.state === "emitida" && " Si ya está timbrada, cancélala también en el portal del SAT."}
              </p>
              <div className="flex flex-wrap gap-2">
                <button type="button" disabled={cancel.pending} onClick={cancel.confirm} className={dangerButton}>
                  {cancel.pending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
                  Sí, cancelar factura
                </button>
                <button type="button" disabled={cancel.pending} onClick={cancel.dismiss} className={secondaryButton}>
                  No, conservarla
                </button>
              </div>
            </div>
          ) : (
            <button type="button" onClick={cancel.ask} className={dangerButton}>
              <Ban className="h-4 w-4" aria-hidden="true" />
              Cancelar factura
            </button>
          )}
        </div>
      )}
    </div>
  );
}
