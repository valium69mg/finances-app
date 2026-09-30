import type { Client } from "../../api/settings";
import type { InvoiceState, InvoiceWarning, Periodicity } from "../../api/invoices";
import { formatMoney } from "../expenses/money";

/** Client whose invoices are an export of services in USD (the backend keys on this ID). */
export const USA_CLIENT_ID = "usa";

export const isUsaClient = (id: string) => id === USA_CLIENT_ID;

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
    default:
      return w.message;
  }
}
