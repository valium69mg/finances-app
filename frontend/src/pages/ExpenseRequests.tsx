import { useAuth } from "../auth/AuthContext";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";
import { HouseholdView } from "./expenserequests/HouseholdView";
import { OwnerView } from "./expenserequests/OwnerView";

/**
 * Peticiones de gasto. The household role asks for an expense and follows its state; the owner reviews them,
 * approves (as a Gasto or a future expense, with the budget check in front) or rejects with a comment.
 */
export function ExpenseRequests() {
  const { role } = useAuth();
  const owner = role === "owner";
  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/peticiones")}>Peticiones</PageTitle>
      <p className="mt-1 text-sm text-muted">
        {owner
          ? "Revisa lo que te piden: apruébalo como gasto (viendo cómo queda el presupuesto), muévelo a gastos futuros o recházalo con un comentario."
          : "Pide un gasto con su monto y descripción. El titular lo aprueba o lo rechaza y te avisamos por correo; mientras siga solicitada puedes cancelarla."}
      </p>
      {owner ? <OwnerView /> : <HouseholdView />}
    </section>
  );
}
