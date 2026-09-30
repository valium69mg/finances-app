import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/** Maps a savings API failure to Spanish copy; the backend `message` is shown for invalid_saving and invalid_valuation. */
export function describeSavingsError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "invalid_saving" || err.code === "invalid_valuation") return `Los datos no son válidos: ${err.message}`;
    if (err.status === 404) return "El ahorro ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
