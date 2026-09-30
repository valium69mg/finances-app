import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps a future expenses API failure to Spanish copy. The backend `message` is appended only for the
 * validation codes where it says what is wrong; everything else falls back to the generic save error.
 */
export function describeFutureError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_future_expense":
        return `Los datos no son válidos: ${err.message}`;
      case "invalid_movement":
        return `No se pudo registrar el movimiento: ${err.message}`;
      case "already_paid":
        return "Este gasto futuro ya está pagado. Actualiza la lista para verlo.";
      case "insufficient_free_balance":
        return "El saldo libre no alcanza para esa cantidad. Actualiza la lista e intenta con un monto menor.";
    }
    if (err.status === 404) return "El gasto futuro ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
