import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/** Maps an expenses API failure to Spanish copy; the backend `message` is shown for invalid_expense. */
export function describeExpenseError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "invalid_expense") return `Los datos no son válidos: ${err.message}`;
    if (err.status === 404) return "El gasto ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}
