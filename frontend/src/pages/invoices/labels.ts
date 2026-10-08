import type { Client } from "../../api/settings";
import type { InvoiceState, InvoiceWarning, Periodicity } from "../../api/invoices";
import { formatMoney } from "../expenses/money";

/** Client whose invoices are an export of services in USD (the backend keys on this ID). */
export const USA_CLIENT_ID = "usa";

export const isUsaClient = (id: string) => id === USA_CLIENT_ID;

/** Generic RFC of público en general: the only receiver of a global invoice. */
export const PUBLIC_GENERAL_RFC = "XAXX010101000";
/** Generic RFCs whose receiver postal code is the issuer's. */
export const GENERIC_RFCS = [PUBLIC_GENERAL_RFC, "XEXX010101000"];

/** True when a decimal string is zero ("0", "0.00", ""). */
export const isZeroAmount = (v: string) => !/[1-9]/.test(v);

export const STATE_LABEL: Record<InvoiceState, string> = {
  preparada: "Preparada",
  emitida: "Emitida",
  cancelada: "Cancelada",
};

export const PERIODICITY_LABEL: Record<Periodicity, string> = {
  mensual: "04 Mensual",
  quincenal: "03 Quincenal",
};

export const MISSING_CONFIG_LABEL: Record<string, string> = {
  "issuer.rfc": "RFC del emisor",
  "issuer.name": "nombre del emisor",
  "issuer.postal_code": "código postal del emisor",
  "client.postal_code": "código postal del cliente",
};

const AMOUNT_FIELD_LABEL: Record<string, string> = {
  subtotal: "Subtotal",
  iva: "IVA",
  isr_withheld: "Retención de ISR",
  iva_withheld: "Retención de IVA",
  total: "Total",
};

export function clientName(clients: Client[], id: string): string {
  return clients.find((c) => c.id === id)?.name ?? id;
}

/** "$3,500.00 USD". */
export const money = (value: string, currency: string) => `${formatMoney(value)} ${currency}`;

/** Spanish text of a warning; `currency` formats the amounts of the mismatch warnings. */
export function describeWarning(w: InvoiceWarning, currency = ""): string {
  const amount = (v?: string) => (v ? (currency ? money(v, currency) : formatMoney(v)) : "");
  switch (w.code) {
    case "possible_duplicate": {
      const ids = (w.invoice_ids ?? []).map((id) => `#${id}`).join(", ");
      return `Ya existe una factura de este cliente con la misma fecha de cobro (${ids}). Revisa que no la estés duplicando.`;
    }
    case "total_mismatch":
      return `El total del XML (${amount(w.actual)}) no coincide con el de la factura preparada (${amount(w.expected)}). Se guardó de todos modos: confirma que el XML es el correcto.`;
    case "subtotal_mismatch":
      return `El subtotal del XML (${amount(w.actual)}) no coincide con el de la factura preparada (${amount(w.expected)}).`;
    case "currency_mismatch":
      return `La moneda del XML (${w.actual ?? "?"}) no coincide con la de la factura preparada (${w.expected ?? "?"}).`;
    case "amounts_from_xml": {
      const changes = (w.changes ?? []).map((c) => `${AMOUNT_FIELD_LABEL[c.field] ?? c.field}: ${amount(c.from)} → ${amount(c.to)}`).join("; ");
      return `Los importes de la factura se tomaron del XML timbrado${changes ? ` (${changes})` : ""}.`;
    }
    case "period_already_filed":
      return "El periodo de esta factura ya fue declarado. La factura no quedó incluida en esa declaración y su ingreso aún no está declarado: decláralo con el SAT fuera de esta app. Aparece en \"Declaraciones presentadas\" como factura sin declarar.";
    default:
      return w.message;
  }
}
