import { useCategoryNameParam } from "../hooks/useUrlParams";

/**
 * The owner's category filter of Gastos and Ahorros: `?categoria=<name>` counts only when it is one of the settings'
 * categories of that kind (anything else is ignored, no chip and no filtering). While the settings load a link's
 * filter is `pending`, so the list waits instead of flashing unfiltered rows.
 */
export function useCategoryFilter(kind: "Gasto" | "Ahorro", categories: { kind: string; name: string }[] | undefined, loading: boolean) {
  const [raw, setRaw] = useCategoryNameParam();
  const filter = categories?.some((c) => c.kind === kind && c.name === raw) ? raw : "";
  return { filter, pending: raw !== "" && loading, clear: () => setRaw("") };
}
