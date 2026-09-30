import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/** Maps an income API failure to Spanish copy; the backend `message` is shown for invalid_income. */
export function describeIncomeError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "invalid_income") return `Los datos no son válidos: ${err.message}`;
    if (err.status === 404) return "El ingreso ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
