import { PERSONAL_PAGES } from "./personal/config";
import { PersonalMovements } from "./personal/PersonalMovements";

/** Household only: her own savings, contributions and withdrawals (categories are fixed). */
export function MisAhorros() {
  return <PersonalMovements config={PERSONAL_PAGES.Ahorro} />;
}
