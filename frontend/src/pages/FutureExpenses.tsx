import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Loader2, Target } from "lucide-react";
import { futureExpensesKeys, listFutureExpenses, type FutureExpenseItem, type PayFutureResult } from "../api/futureExpenses";
import { getSettings, settingsKeys } from "../api/settings";
import { EmptyNote } from "../components/EmptyNote";
import { InlineFormPanel } from "../components/InlineFormPanel";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";
import { formatMoney } from "./expenses/money";
import { AmountDialog, type AmountMode } from "./futureexpenses/AmountDialog";
import { describeFutureError } from "./futureexpenses/errors";
import { ItemForm } from "./futureexpenses/ItemForm";
import { ActiveList, PaidList } from "./futureexpenses/ItemLists";
import { PayDialog } from "./futureexpenses/PayDialog";
import { ErrorBanner, secondaryButton } from "./settings/ui";

type Dialog = { kind: "amount"; mode: AmountMode; item: FutureExpenseItem } | { kind: "pay"; item: FutureExpenseItem } | null;

/**
 * Gastos futuros: the things the owner chooses to save for. Registering, editing and deleting them, adding savings,
 * assigning the free balance and "Marcar pagado" all live on this page.
 */
export function FutureExpenses() {
  const [editing, setEditing] = useState<FutureExpenseItem | null>(null);
  // The create form stays collapsed until asked for; editing a row opens it by itself.
  const [formOpen, setFormOpen] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const list = useQuery({ queryKey: futureExpensesKeys.list, queryFn: listFutureExpenses, retry: false });
  // Only the "Marcar pagado" category picker needs the settings, so the page never waits for them.
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const categories = (settings.data?.categories ?? []).filter((c) => c.kind === "Gasto").map((c) => c.name);

  function onPaid({ item, expense }: PayFutureResult) {
    setDialog(null);
    setEditing((cur) => (cur?.id === item.id ? null : cur));
    setNotice(`${item.name} se marcó como pagado: se registró un gasto de ${formatMoney(expense.amount)} en ${expense.category} y se liberó su ahorro.`);
  }

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/gastos-futuros")}>Gastos futuros</PageTitle>
      <p className="mt-1 text-sm text-muted">
        Registra lo que quieres ir juntando (un seguro, un viaje, una compra grande) con su monto y fecha. Cada ahorro que agregas aquí queda ligado a ese gasto, y te
        sugerimos cuánto apartar en cada ciclo de pago para llegar a tiempo.
      </p>

      <div className="mt-6 space-y-6 rounded-xl border border-border bg-surface p-4 sm:p-6">

        {list.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando gastos futuros…
          </p>
        )}
        {list.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeFutureError(list.error)}</ErrorBanner>
            <button type="button" onClick={() => void list.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}

        {list.data && (
          <>
            <section aria-label="Saldo libre" className="rounded-lg border border-border p-4">
              <p className="text-sm text-muted">Saldo libre</p>
              <p className="text-xl font-semibold tabular-nums">{formatMoney(list.data.free_balance)}</p>
              <p className="mt-1 text-sm text-muted">
                Ahorro en la categoría Gastos futuros que aún no está asignado a ningún gasto. No se reparte solo: tú decides cuánto asignar a cada uno.
              </p>
            </section>

            <InlineFormPanel id="future-form-panel" label="+ Nuevo gasto futuro" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={editing !== null}>
              <ItemForm
                key={editing?.id ?? "new"}
                editing={editing}
                onSaved={(item, wasEdit) => {
                  setEditing(null);
                  setFormOpen(false);
                  setNotice(wasEdit ? `Cambios de ${item.name} guardados.` : `${item.name} agregado. Aparta ${formatMoney(item.suggested_monthly)} al mes para llegar a tiempo.`);
                }}
                onCancelEdit={() => {
                  setEditing(null);
                  setFormOpen(false);
                }}
              />
            </InlineFormPanel>

            {notice && (
              <p role="status" className="flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                <span className="min-w-0 break-words">{notice}</span>
              </p>
            )}

            <div className="space-y-3">
              <h3 className="text-base font-semibold tracking-tight">Por juntar</h3>
              {list.data.active.length === 0 ? (
                <EmptyNote icon={Target}>Aún no tienes gastos futuros. Agrega el primero con el botón de arriba.</EmptyNote>
              ) : (
                <>
                  <ActiveList
                    items={list.data.active}
                    freeBalance={list.data.free_balance}
                    editingId={editing?.id ?? null}
                    onSaving={(item) => {
                      setNotice(null);
                      setDialog({ kind: "amount", mode: "saving", item });
                    }}
                    onAssign={(item) => {
                      setNotice(null);
                      setDialog({ kind: "amount", mode: "assign", item });
                    }}
                    onPay={(item) => {
                      setNotice(null);
                      setDialog({ kind: "pay", item });
                    }}
                    onEdit={(item) => {
                      setNotice(null);
                      setEditing(item);
                    }}
                    onDeleted={(item) => {
                      setEditing((cur) => (cur?.id === item.id ? null : cur));
                      setNotice(`${item.name} se eliminó. Su ahorro volvió al saldo libre.`);
                    }}
                  />
                  <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm tabular-nums">
                    <dt className="font-medium">Total por juntar</dt>
                    <dd className="text-right font-semibold">{formatMoney(list.data.totals.target)}</dd>
                    <dt className="text-muted">Ahorrado</dt>
                    <dd className="text-right">{formatMoney(list.data.totals.saved)}</dd>
                    <dt className="text-muted">Sugerido al mes</dt>
                    <dd className="text-right">{formatMoney(list.data.totals.suggested_monthly)}</dd>
                  </dl>
                </>
              )}
            </div>

            {list.data.paid.length > 0 && (
              <div className="space-y-3">
                <h3 className="text-base font-semibold tracking-tight">Pagados</h3>
                <PaidList
                  items={list.data.paid}
                  onDeleted={(item) => setNotice(`${item.name} se quitó del historial. El gasto que registró se conserva.`)}
                />
              </div>
            )}
          </>
        )}

        {dialog?.kind === "amount" && (
          <AmountDialog
            item={dialog.item}
            mode={dialog.mode}
            freeBalance={list.data?.free_balance ?? "0"}
            onClose={() => setDialog(null)}
            onDone={(message) => {
              setDialog(null);
              setNotice(message);
            }}
          />
        )}
        {dialog?.kind === "pay" && <PayDialog item={dialog.item} categories={categories} onClose={() => setDialog(null)} onPaid={onPaid} />}
      </div>
    </section>
  );
}
