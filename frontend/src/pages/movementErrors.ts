import { ApiError } from "../api/client";
import { describeSaveError } from "./settings/ui";

/** Same limit as the backend (ledger.MaxDescriptionLength) and the movements table. */
export const MAX_MOVEMENT_DESCRIPTION_LENGTH = 200;

interface MovementErrorOptions {
  /** Backend error code whose `message` is worth showing, e.g. "invalid_expense". */
  invalidCode: string;
  /** What the request was about, with its article: "el gasto", "la valuación". */
  noun: string;
}

/**
 * Maps a movement API failure (expenses, income, savings, valuations) to Spanish
 * copy: the backend `message` for the module's validation code, a "no longer
 * exists" hint for a 404 and the generic save error for anything else.
 */
export function describeMovementError(err: unknown, { invalidCode, noun }: MovementErrorOptions): string {
  if (err instanceof ApiError) {
    if (err.code === invalidCode) return `Los datos no son válidos: ${err.message}`;
    if (err.status === 404) return `${noun.charAt(0).toUpperCase()}${noun.slice(1)} ya no existe. Actualiza la lista e intenta de nuevo.`;
  }
  return describeSaveError(err);
}
