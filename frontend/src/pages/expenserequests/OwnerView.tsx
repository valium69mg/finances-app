import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, ClipboardList, Loader2, Undo2, XCircle } from "lucide-react";
import { expenseRequestsKeys, getRequestCategories, listExpenseRequests, REQUEST_STATUSES, type ApproveResult, type ExpenseRequest, type RequestStatus } from "../../api/expenseRequests";
import { getSettings, settingsKeys } from "../../api/settings";
import { EmptyNote } from "../../components/EmptyNote";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, primaryButton, secondaryButton } from "../settings/ui";
import { ApproveDialog } from "./ApproveDialog";
import { describeRequestError } from "./errors";
import { STATUS_META } from "./labels";
import { RejectDialog } from "./RejectDialog";
import { RevertDialog } from "./RevertDialog";
import { RequestCard } from "./RequestCard";

type Filter = RequestStatus | "";
type Dialog = { kind: "approve" | "reject" | "revert"; request: ExpenseRequest } | null;

const FILTERS: { value: Filter; label: string }[] = [
  ...REQUEST_STATUSES.map((s) => ({ value: s as Filter, label: `${STATUS_META[s].label}s` })),
  { value: "", label: "Todas" },
];

/** One-line summary of what an approval did, including the budget feedback of a Gasto. */
function approvedNotice({ request: r, budget }: ApproveResult): string {
  if (r.result_kind === "gasto_futuro") return `Aprobaste ${r.description}: se movió a gastos futuros.`;
  const base = `Aprobaste ${r.description}: se registró un gasto de ${formatMoney(r.amount)}`;
  if (!budget) return `${base}.`;
  if (budget.budget === null) return `${base} en ${budget.category} (esa categoría no tiene presupuesto).`;
  if (budget.over_budget) return `${base} en ${budget.category}, que ahora excede su presupuesto (gastado ${formatMoney(budget.spent)} de ${formatMoney(budget.budget)}).`;
  return `${base} en ${budget.category}: gastado ${formatMoney(budget.spent)} de ${formatMoney(budget.budget)}.`;
}

/** What the owner sees: every request with filters by state, and Aprobar / Rechazar on the pending ones. */
export function OwnerView() {
  const [filter, setFilter] = useState<Filter>("solicitada");
  const [dialog, setDialog] = useState<Dialog>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const list = useQuery({ queryKey: expenseRequestsKeys.list(filter), queryFn: () => listExpenseRequests(filter), retry: false });
  const categories = useQuery({ queryKey: expenseRequestsKeys.categories, queryFn: getRequestCategories, retry: false });
  // Only the payment method picker needs the settings, so the page never waits for them.
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  return (
    <div className="mt-6 space-y-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
      <div role="group" aria-label="Filtrar por estado" className="flex flex-wrap gap-2">
        {FILTERS.map((f) => (
          <button
            key={f.label}
            type="button"
            aria-pressed={filter === f.value}
            onClick={() => setFilter(f.value)}
            className={`focus-ring inline-flex min-h-11 items-center rounded-lg border px-4 py-2 text-sm font-medium transition-colors duration-200 ${
              filter === f.value ? "border-primary bg-primary/10 text-primary" : "border-border bg-surface hover:bg-primary/5"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <div role="status" aria-live="polite">
        {notice && (
          <p className="flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
            <span className="min-w-0 break-words">{notice}</span>
          </p>
        )}
      </div>

      {list.isPending && (
        <p role="status" className="flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Cargando peticiones…
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
      {list.data &&
        (list.data.length === 0 ? (
          <EmptyNote icon={ClipboardList}>{filter === "solicitada" ? "No hay peticiones pendientes." : "No hay peticiones con este filtro."}</EmptyNote>
        ) : (
          <ul aria-label="Peticiones" className="divide-y divide-border rounded-xl border border-border">
            {list.data.map((r) => (
              <RequestCard
                key={r.id}
                request={r}
                showRequester
                actions={
                  r.status === "solicitada" ? (
                    <>
                      <button
                        type="button"
                        onClick={() => {
                          setNotice(null);
                          setDialog({ kind: "approve", request: r });
                        }}
                        aria-label={`Aprobar la petición ${r.description}`}
                        className={primaryButton}
                      >
                        <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                        Aprobar
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setNotice(null);
                          setDialog({ kind: "reject", request: r });
                        }}
                        aria-label={`Rechazar la petición ${r.description}`}
                        className={dangerButton}
                      >
                        <XCircle className="h-4 w-4" aria-hidden="true" />
                        Rechazar
                      </button>
                    </>
                  ) : r.status === "aprobada" ? (
                    <button
                      type="button"
                      onClick={() => {
                        setNotice(null);
                        setDialog({ kind: "revert", request: r });
                      }}
                      aria-label={`Volver a solicitada la petición ${r.description}`}
                      className={secondaryButton}
                    >
                      <Undo2 className="h-4 w-4" aria-hidden="true" />
                      Volver a solicitada
                    </button>
                  ) : undefined
                }
              />
            ))}
          </ul>
        ))}

      {dialog?.kind === "approve" && (
        <ApproveDialog
          key={dialog.request.id}
          request={dialog.request}
          categories={categories.data ?? []}
          paymentMethods={settings.data?.payment_methods ?? []}
          onClose={() => setDialog(null)}
          onApproved={(result) => {
            setDialog(null);
            setNotice(approvedNotice(result));
          }}
        />
      )}
      {dialog?.kind === "reject" && (
        <RejectDialog
          key={dialog.request.id}
          request={dialog.request}
          onClose={() => setDialog(null)}
          onRejected={(r) => {
            setDialog(null);
            setNotice(`Rechazaste ${r.description}. Quien la pidió recibirá tu comentario.`);
          }}
        />
      )}
      {dialog?.kind === "revert" && (
        <RevertDialog
          key={dialog.request.id}
          request={dialog.request}
          onClose={() => setDialog(null)}
          onReverted={(r) => {
            setDialog(null);
            setNotice(`${r.description} volvió a solicitada: la aprobación se deshizo.`);
          }}
        />
      )}
    </div>
  );
}
