import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { getSettings, settingsKeys } from "../api/settings";
import { FutureExpensesView } from "./futureexpenses/FutureExpensesView";

/**
 * Gastos futuros of the owner: the things the owner chooses to save for (family ledger). The page itself is shared
 * with "Mis gastos futuros" (see FutureExpensesView).
 */
export function FutureExpenses() {
  // Only the "Marcar pagado" category picker needs the settings, so the page never waits for them.
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const categories = useMemo(
    () => (settings.data?.categories ?? []).filter((c) => c.kind === "Gasto").map((c) => ({ value: c.name, label: c.name })),
    [settings.data],
  );
  return <FutureExpensesView path="/gastos-futuros" title="Gastos futuros" categories={categories} />;
}
