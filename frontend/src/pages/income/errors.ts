import { describeMovementError } from "../movementErrors";

/** Maps an income API failure to Spanish copy; the backend `message` is shown for invalid_income. */
export function describeIncomeError(err: unknown): string {
  return describeMovementError(err, { invalidCode: "invalid_income", noun: "el ingreso" });
}
