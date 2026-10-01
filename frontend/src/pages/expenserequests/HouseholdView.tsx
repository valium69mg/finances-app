import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { CheckCircle2, ClipboardList, Loader2, Undo2 } from "lucide-react";
import { cancelExpenseRequest, expenseRequestsKeys, getRequestCategories, listExpenseRequests, type ExpenseRequest } from "../../api/expenseRequests";
import { EmptyNote } from "../../components/EmptyNote";
import { InlineFormPanel } from "../../components/InlineFormPanel";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeRequestError } from "./errors";
import { RequestCard } from "./RequestCard";
import { RequestForm } from "./RequestForm";
import { useInvalidateRequests } from "./useInvalidate";

/** What the household role sees: a form to ask for an expense and the state of her own requests. */
export function HouseholdView() {
  const invalidate = useInvalidateRequests();
  const [formOpen, setFormOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const list = useQuery({ queryKey: expenseRequestsKeys.list(""), queryFn: () => listExpenseRequests(), retry: false });
  const categories = useQuery({ queryKey: expenseRequestsKeys.categories, queryFn: getRequestCategories, retry: false });

  const cancel = useMutation({
    mutationFn: (r: ExpenseRequest) => cancelExpenseRequest(r.id),
    onSuccess: async (_, r) => {
      setNotice(`Cancelaste tu petición de ${formatMoney(r.amount)} (${r.description}).`);
      await invalidate();
    },
    // The list is the truth: a request decided meanwhile shows its real state after the refresh.
    onError: () => void invalidate(),
  });

  return (
    <div className="mt-6 space-y-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
      <InlineFormPanel id="request-form-panel" label="+ Nueva petición" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={false}>
        <RequestForm
          categories={categories.data ?? []}
          onSaved={(r) => {
            setFormOpen(false);
            setNotice(`Petición enviada: ${formatMoney(r.amount)} (${r.description}). Te avisaremos por correo cuando se resuelva.`);
          }}
        />
      </InlineFormPanel>

      <div role="status" aria-live="polite">
        {notice && (
          <p className="flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
            <span className="min-w-0 break-words">{notice}</span>
          </p>
        )}
      </div>

      <div className="space-y-3">
        <h3 className="text-base font-semibold tracking-tight">Mis peticiones</h3>
        {list.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando tus peticiones…
          </p>
        )}
        {list.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeRequestError(list.error)}</ErrorBanner>
            <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {cancel.isError && <ErrorBanner>{describeRequestError(cancel.error)}</ErrorBanner>}
        {list.data &&
          (list.data.length === 0 ? (
            <EmptyNote icon={ClipboardList}>Aún no has hecho ninguna petición. Crea la primera con el botón de arriba.</EmptyNote>
          ) : (
            <ul aria-label="Mis peticiones" className="divide-y divide-border rounded-xl border border-border">
              {list.data.map((r) => (
                <RequestCard
                  key={r.id}
                  request={r}
                  actions={
                    r.status === "solicitada" ? (
                      <button
                        type="button"
                        onClick={() => {
                          setNotice(null);
                          cancel.mutate(r);
                        }}
                        disabled={cancel.isPending}
                        aria-label={`Cancelar la petición ${r.description}`}
                        className={dangerButton}
                      >
                        <Undo2 className="h-4 w-4" aria-hidden="true" />
                        Cancelar
                      </button>
                    ) : undefined
                  }
                />
              ))}
            </ul>
          ))}
      </div>
    </div>
  );
}
