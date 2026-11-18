import type { Saving } from "../../api/savings";

/**
 * A future expense assignment is a transfer group (two rows sharing a transfer_id) made of "Asignado a <item>" rows of
 * which one feeds the item. Once the item is deleted neither row is linked any more, so the description is the fallback.
 */
export function isAssignment(s: Saving): boolean {
  return Boolean(s.transfer_id) && (s.future_expense_id != null || s.description.startsWith("Asignado a "));
}
