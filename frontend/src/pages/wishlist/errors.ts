import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps a wishlist API failure to Spanish copy. The backend `message` is appended only for the validation codes where
 * it says what is wrong; everything else falls back to the generic save error. The request conversion shares the
 * limit of the Peticiones page (rate_limited).
 */
export function describeWishError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_wishlist_item":
      case "invalid_future_expense":
      case "invalid_personal":
      case "invalid_expense_request":
        return `Los datos no son válidos: ${err.message}`;
      case "already_converted":
        return "Este deseo ya se convirtió. Actualiza la lista para ver en qué.";
      case "rate_limited":
        return "Enviaste demasiadas peticiones seguidas. Espera un rato e intenta de nuevo.";
    }
    if (err.status === 404) return "El deseo ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
