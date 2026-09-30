import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps a bills API failure to Spanish copy. The backend `message` is only
 * appended for the validation codes where it says what is wrong; everything
 * else falls back to the generic save error (session, server, network,
 * incomplete tax settings).
 */
export function describeBillsError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_bill":
        return `Los datos no son válidos: ${err.message}`;
      case "invalid_expense":
        return `No se pudo registrar el gasto: ${err.message}`;
      case "occurrence_resolved":
        return "Este vencimiento ya se pagó u omitió. Actualiza la lista para ver el siguiente.";
      case "bill_inactive":
        return "El pago recurrente está desactivado. Reactívalo para pagarlo u omitirlo.";
    }
    if (err.status === 404) return "El pago recurrente ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
