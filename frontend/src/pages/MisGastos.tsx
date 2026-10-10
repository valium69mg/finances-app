import { PERSONAL_PAGES } from "./personal/config";
import { PersonalMovements } from "./personal/PersonalMovements";

/** Household only: her own expenses, with her own categories (add and rename). */
export function MisGastos() {
  return <PersonalMovements config={PERSONAL_PAGES.Gasto} manageCategories />;
}
