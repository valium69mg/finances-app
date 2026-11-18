import { formatMoney } from "../pages/expenses/money";

/** "Pagado con ahorro", with the covered amount when the savings paid only part of the expense. */
export function coveredText(covered: string | undefined, amount: string): string | null {
  if (!covered || Number(covered) <= 0) return null;
  return Number(covered) >= Number(amount) ? "Pagado con ahorro" : `Pagado con ahorro: ${formatMoney(covered)}`;
}

/** Badge of an expense paid from the savings of a future expense; renders nothing when savings paid none of it. */
export function CoveredBadge({ covered, amount }: { covered: string | undefined; amount: string }) {
  const text = coveredText(covered, amount);
  if (!text) return null;
  return <span className="mt-1 inline-block rounded-full border border-primary/40 bg-primary/10 px-2 py-0.5 text-xs font-medium">{text}</span>;
}
