import type { PeriodFilingStatus, TaxWarning } from "../../api/taxFiling";

/** Spanish text of a period's filing state. */
export const FILING_STATUS_LABEL: Record<PeriodFilingStatus, string> = {
  ninguna: "Sin declarar",
  pendiente: "Pago pendiente",
  pagada: "Pagada",
};

const MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];

/** "2026-10" -> "octubre de 2026"; anything that is not YYYY-MM is returned untouched. */
export function periodLabel(period: string): string {
  const m = /^(\d{4})-(0[1-9]|1[0-2])$/.exec(period);
  return m ? `${MONTHS[Number(m[2]) - 1]} de ${m[1]}` : period;
}

/** "2026-11-17" -> "17 de noviembre de 2026". */
export function dateLabel(date: string): string {
  const m = /^(\d{4})-(0[1-9]|1[0-2])-(\d{2})$/.exec(date);
  return m ? `${Number(m[3])} de ${MONTHS[Number(m[2]) - 1]} de ${m[1]}` : date;
}

/** Previous month of a YYYY-MM period (December of the previous year for January). */
export function previousPeriod(period: string): string {
  const m = /^(\d{4})-(\d{2})$/.exec(period);
  if (!m) return period;
  const [year, month] = [Number(m[1]), Number(m[2])];
  return month === 1 ? `${year - 1}-12` : `${year}-${String(month - 1).padStart(2, "0")}`;
}

/** Spanish text of a preview or registration warning. */
export function describeTaxWarning(w: TaxWarning): string {
  switch (w.code) {
    case "prepared_invoices": {
      const ids = (w.invoice_ids ?? []).map((id) => `#${id}`).join(", ");
      return `El periodo tiene facturas que aún están en estado "preparada" (${ids}). No cuentan en la declaración hasta que las marques como emitidas.`;
    }
    case "already_filed":
      return "Este periodo ya fue declarado. Lo que ves es un cálculo con las facturas de hoy; no cambia la declaración registrada.";
    default:
      return w.message;
  }
}
