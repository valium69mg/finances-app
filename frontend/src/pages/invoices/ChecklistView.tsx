import type { ReactNode } from "react";
import { ClipboardList } from "lucide-react";
import type { Checklist } from "../../api/invoices";
import { formatMoney } from "../expenses/money";
import { GENERIC_RFCS, MISSING_CONFIG_LABEL, PERIODICITY_LABEL, isZeroAmount, money } from "./labels";

const CONFIRM_TAG = "confirmar con contador";

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="rounded-lg border border-border p-4">
      <h4 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted">{title}</h4>
      <dl className="grid gap-x-4 gap-y-2 text-sm sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">{children}</dl>
    </section>
  );
}

interface RowProps {
  label: string;
  value: string;
  /** An empty value is shown as a pending item the user must configure. */
  confirm?: boolean;
  note?: string;
}

function Row({ label, value, confirm, note }: RowProps) {
  return (
    <>
      <dt className="text-muted">{label}</dt>
      <dd className="min-w-0 break-words">
        {value === "" ? <span className="font-medium text-destructive">Pendiente (complétalo en Configuración)</span> : <span className="font-medium">{value}</span>}
        {note && <span className="text-muted"> {note}</span>}
        {confirm && <span className="ml-2 inline-block rounded-full border border-border px-2 py-0.5 text-xs text-muted">{CONFIRM_TAG}</span>}
      </dd>
    </>
  );
}

/** The data to type into the SAT portal "Genera tu factura" form. The app does not stamp. */
export function ChecklistView({ checklist: c }: { checklist: Checklist }) {
  const confirms = new Set(c.to_confirm);
  const v = c.voucher;
  const g = v.global;
  const retained = !isZeroAmount(c.taxes.isr_withheld) || !isZeroAmount(c.taxes.iva_withheld);
  const missing = c.missing_config.map((k) => MISSING_CONFIG_LABEL[k] ?? k);

  return (
    <div className="space-y-3">
      <div>
        <h3 className="flex items-center gap-2 text-base font-semibold tracking-tight">
          <ClipboardList className="h-5 w-5 shrink-0" aria-hidden="true" />
          Checklist para el portal del SAT
        </h3>
        <p className="mt-1 text-sm text-muted">
          Captura estos datos en «Genera tu factura» del portal del SAT. Los marcados «{CONFIRM_TAG}» dependen de tu caso.
        </p>
      </div>

      {missing.length > 0 && (
        <p role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          Faltan datos: {missing.join(", ")}. Complétalos en Configuración.
        </p>
      )}

      <Section title="Emisor">
        <Row label="RFC" value={c.issuer.rfc} />
        <Row label="Nombre" value={c.issuer.name} />
        <Row label="Régimen fiscal" value={c.issuer.regimen} />
        <Row label="Lugar de expedición (CP)" value={c.issuer.postal_code} />
      </Section>

      <Section title="Receptor">
        <Row label="RFC" value={c.receiver.rfc} />
        <Row label="Nombre" value={c.receiver.name} />
        <Row label="Régimen fiscal receptor" value={c.receiver.regimen} />
        <Row label="CP receptor" value={c.receiver.postal_code} note={GENERIC_RFCS.includes(c.receiver.rfc) ? "(mismo del emisor)" : undefined} />
        <Row label="Uso de CFDI" value={c.receiver.uso_cfdi ?? ""} />
        {c.receiver.internal_note && <Row label="Nota interna" value={c.receiver.internal_note} note="(no va en el CFDI)" />}
      </Section>

      <Section title="Comprobante">
        <Row label="Tipo" value={`${v.type} (Ingreso)`} />
        <Row label="Moneda" value={v.currency} />
        {v.exchange_rate && (
          <Row label="Tipo de cambio" value={v.exchange_rate} confirm={confirms.has("fx_rate_dof")} note="(usa el tipo de cambio FIX publicado en el DOF para la fecha de cobro)" />
        )}
        <Row label="Forma de pago" value={`${v.payment_form} Transferencia electrónica de fondos`} />
        <Row label="Método de pago" value={v.payment_method} />
        {v.export && <Row label="Exportación" value="Clave de exportación, venta a tasa 0%" confirm={confirms.has("export_key")} />}
        {g && (
          <>
            <Row label="InformacionGlobal · periodicidad" value={PERIODICITY_LABEL[g.periodicity] ?? g.code} confirm={confirms.has("global_info")} />
            <Row label="InformacionGlobal · meses" value={g.months} />
            <Row label="InformacionGlobal · año" value={g.year} />
          </>
        )}
      </Section>

      <Section title="Concepto">
        <Row label="Clave de producto o servicio" value={c.concept.prod_serv_key} confirm={confirms.has("prod_serv_key")} />
        <Row label="Clave de unidad" value={c.concept.unit_key} confirm={confirms.has("unit_key")} />
        <Row label="Descripción" value={c.concept.description} />
        <Row label="Cantidad" value={String(c.concept.quantity)} />
        <Row label="Valor unitario" value={money(c.concept.unit_value, v.currency)} />
        {v.export && <Row label="Objeto de impuesto" value="Según tu caso" confirm={confirms.has("tax_object")} />}
      </Section>

      <Section title="Impuestos">
        {c.taxes.iva_included ? (
          <Row
            label="Traslado de IVA"
            value={`${formatMoney(c.taxes.iva)} MXN`}
            note={retained ? "(se suma al subtotal; el monto recibido ya descuenta las retenciones)" : "(incluido en el monto recibido)"}
          />
        ) : (
          <Row label="Traslado de IVA" value="Tasa 0% = $0.00" note="(exportación de servicios)" />
        )}
        {retained ? (
          <>
            {!isZeroAmount(c.taxes.isr_withheld) && <Row label="Retención de ISR" value={`${formatMoney(c.taxes.isr_withheld)} MXN`} />}
            {!isZeroAmount(c.taxes.iva_withheld) && <Row label="Retención de IVA" value={`${formatMoney(c.taxes.iva_withheld)} MXN`} />}
          </>
        ) : (
          <Row label="Retenciones" value="Ninguna" />
        )}
      </Section>

      <Section title="Totales">
        <Row label="Subtotal" value={money(c.totals.subtotal, c.totals.currency)} />
        <Row label="Total" value={money(c.totals.total, c.totals.currency)} />
        <Row label="Depósito esperado" value={money(c.totals.expected_deposit_mxn, "MXN")} />
      </Section>

      <p className="text-sm text-muted">
        Periodo fiscal: <span className="font-medium text-foreground">{c.period}</span>. Fecha límite de declaración de ese periodo:{" "}
        <span className="font-medium text-foreground">{c.due_date}</span> (si cae en fin de semana o feriado, se recorre al siguiente día hábil).
      </p>
    </div>
  );
}
