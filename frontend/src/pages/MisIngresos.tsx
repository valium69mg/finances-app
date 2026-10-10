import { PERSONAL_PAGES } from "./personal/config";
import { PersonalMovements } from "./personal/PersonalMovements";

/** Household only: her own income (categories are fixed). */
export function MisIngresos() {
  return <PersonalMovements config={PERSONAL_PAGES.Ingreso} />;
}
