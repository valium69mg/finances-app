import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps a tax filing API failure to Spanish copy. The backend `message` is only
 * appended for the validation codes where it says what is wrong; everything
 * else falls back to the generic save error (session, server, network,
 * incomplete tax settings).
 */
export function describeTaxFilingError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_filing":
        return `Los datos no son válidos: ${err.message}`;
      case "invalid_expense":
        return `No se pudo registrar el gasto de Impuestos: ${err.message}`;
      case "already_filed":
        return "Este periodo ya está declarado. Búscalo en Declaraciones presentadas.";
      case "already_paid":
        return "El pago de esta declaración ya está registrado.";
      case "filing_paid":
        return "Una declaración con el pago registrado no se puede eliminar.";
      case "invoices_changed":
        return "Las facturas del periodo cambiaron mientras registrabas. Vuelve a calcular la declaración e intenta de nuevo.";
    }
    if (err.status === 404) return "La declaración ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
