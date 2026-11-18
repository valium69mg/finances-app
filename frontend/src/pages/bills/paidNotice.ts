import type { PayResult } from "../../api/bills";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

/** Confirmation after a bill payment: what was registered and when the bill is due next. */
export function paidNotice({ bill, expense }: PayResult): string {
  return `Pago de ${bill.name} registrado: ${formatMoney(expense.amount)} ${expense.currency} en ${expense.category}. Próximo vencimiento: ${dateLabel(bill.next_due_date)}.`;
}
