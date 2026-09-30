import { describeMovementError } from "../movementErrors";

/** Maps an expenses API failure to Spanish copy; the backend `message` is shown for invalid_expense. */
export function describeExpenseError(err: unknown): string {
  return describeMovementError(err, { invalidCode: "invalid_expense", noun: "el gasto" });
}
