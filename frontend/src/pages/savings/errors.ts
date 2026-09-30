import { ApiError } from "../../api/client";
import { describeMovementError } from "../movementErrors";

/** Maps a savings or transfer API failure to Spanish copy; the backend `message` is shown for invalid_saving. */
export function describeSavingsError(err: unknown): string {
  if (err instanceof ApiError && err.code === "transfer_leg_locked") {
    return "Las partes de un traspaso no se pueden editar. Elimina el traspaso y regístralo de nuevo.";
  }
  return describeMovementError(err, { invalidCode: "invalid_saving", noun: "el ahorro" });
}

/** Maps a valuation API failure to Spanish copy; the backend `message` is shown for invalid_valuation. */
export function describeValuationError(err: unknown): string {
  return describeMovementError(err, { invalidCode: "invalid_valuation", noun: "la valuación" });
}
