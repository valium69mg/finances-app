import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listPersonalCategories, personalKeys } from "../api/personal";
import { FutureBackendProvider, personalFutureBackend } from "./futureexpenses/backend";
import { FutureExpensesView } from "./futureexpenses/FutureExpensesView";

/**
 * Mis gastos futuros: her own future expenses, the same screen as the owner's over her personal ledger. Paying asks
 * for one of her Gasto categories (required: nothing is inferred from the name).
 */
export function MisGastosFuturos() {
  const gasto = useQuery({ queryKey: personalKeys.categories("Gasto"), queryFn: () => listPersonalCategories("Gasto"), retry: false });
  const categories = useMemo(() => (gasto.data ?? []).map((c) => ({ value: String(c.id), label: c.name })), [gasto.data]);
  return (
    <FutureBackendProvider value={personalFutureBackend}>
      <FutureExpensesView path="/mis-gastos-futuros" title="Mis gastos futuros" categories={categories} categoryRequired />
    </FutureBackendProvider>
  );
}
