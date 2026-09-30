import { describeMovementError } from "../movementErrors";

/** Maps a savings or transfer API failure to Spanish copy; the backend `message` is shown for invalid_saving. */
export function describeSavingsError(err: unknown): string {
  return describeMovementError(err, { invalidCode: "invalid_saving", noun: "el ahorro" });
}

/** Maps a valuation API failure to Spanish copy; the backend `message` is shown for invalid_valuation. */
export function describeValuationError(err: unknown): string {
  return describeMovementError(err, { invalidCode: "invalid_valuation", noun: "la valuación" });
}
