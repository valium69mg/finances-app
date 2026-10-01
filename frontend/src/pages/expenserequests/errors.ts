import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps an expense requests API failure to Spanish copy. The backend `message` is appended only for the
 * validation codes where it says what is wrong; everything else falls back to the generic save error.
 */
export function describeRequestError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_expense_request":
      case "invalid_future_expense":
      case "invalid_expense":
        return `Los datos no son válidos: ${err.message}`;
      case "invalid_state":
        return "Esta petición ya fue resuelta o cancelada. Actualiza la lista para ver su estado.";
      case "future_expense_paid":
        return "No se puede volver a solicitada: ese gasto futuro ya se pagó y se convirtió en un gasto real. Primero deshaz ese pago en Gastos futuros.";
      case "rate_limited":
        return "Enviaste demasiadas peticiones seguidas. Espera un rato e intenta de nuevo.";
    }
    if (err.status === 404) return "La petición ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
