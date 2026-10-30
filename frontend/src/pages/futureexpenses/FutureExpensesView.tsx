import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Loader2, Plus, Target } from "lucide-react";
import type { FutureExpenseItem } from "../../api/futureExpenses";
import { EmptyNote } from "../../components/EmptyNote";
import { DeepLinkNote } from "../../components/DeepLinkNote";
import { Modal } from "../../components/Modal";
import { PageTitle } from "../../components/PageTitle";
import { useDeepLink } from "../../hooks/useDeepLink";
import { moduleIcon } from "../../modules";
import { formatMoney } from "../expenses/money";
import { AmountDialog, type AmountMode } from "./AmountDialog";
import { describeFutureError } from "./errors";
import { ItemForm } from "./ItemForm";
import { ItemDetailDialog, PaidDetailDialog } from "./ItemDetailDialog";
import { ActiveList, PaidList } from "./ItemLists";
import { useFutureBackend, type PaidResult } from "./backend";
import type { PayCategory } from "./form";
import { PayDialog } from "./PayDialog";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";

type Dialog =
  | { kind: "amount"; mode: AmountMode; item: FutureExpenseItem }
  | { kind: "pay"; item: FutureExpenseItem }
  | { kind: "detail"; item: FutureExpenseItem }
  | { kind: "paid"; item: FutureExpenseItem }
  | { kind: "form"; item: FutureExpenseItem | null }
  | null;

interface Props {
  /** Tab path, for the icon. */
  path: string;
  title: string;
  /** Gasto categories offered when paying. */
  categories: PayCategory[];
  /** True when paying needs the category chosen (her ledger infers nothing). */
  categoryRequired?: boolean;
  /** Opens the edit form of the item in `?editar=<id>` (the owner's Panel links to it). Her page leaves it off. */
  deepLinkEdit?: boolean;
}

/** `?editar=<id>`: opens the edit form of that item, or says it is not among the pending ones. Rendered only where the URL is read. */
function EditDeepLink({ items, open, onOpen }: { items: FutureExpenseItem[]; open: FutureExpenseItem | null; onOpen: (item: FutureExpenseItem) => void }) {
  const { missing } = useDeepLink({ param: "editar", items, open, onOpen });
  return missing ? <DeepLinkNote>No se encontró el gasto futuro del enlace entre los pendientes. Puedes buscarlo en la lista.</DeepLinkNote> : null;
}

/**
 * Gastos futuros: the things the user chooses to save for. Summary first: "Nuevo gasto futuro" sits in the header and
 * opens a dialog, each item is a card whose click opens a detail dialog with its actions (add savings, assign the free
 * balance, mark paid, edit, delete), and each action opens its own dialog. The owner's page and "Mis gastos futuros"
 * render it over their own storage through the FutureBackend context.
 */
export function FutureExpensesView({ path, title, categories, categoryRequired = false, deepLinkEdit = false }: Props) {
  const backend = useFutureBackend();
  // The owner's view of a household user: nothing that writes is rendered, and the Hogar page owns the heading.
  const readOnly = backend.readOnly === true;
  const [dialog, setDialog] = useState<Dialog>(null);
  // The item whose edit form is open (the deep link and the edit action both set it through the dialog state).
  const editing = dialog?.kind === "form" ? dialog.item : null;
  const [notice, setNotice] = useState<string | null>(null);
  const list = useQuery({ queryKey: backend.listKey, queryFn: () => backend.list(), retry: false });

  function onPaid({ item, expense }: PaidResult) {
    setDialog(null);
    setNotice(`${item.name} se marcó como pagado: se registró un gasto de ${formatMoney(expense.amount)} en ${expense.category} y se liberó su ahorro.`);
  }

  return (
    <section aria-labelledby={readOnly ? undefined : "page-title"} aria-label={readOnly ? title : undefined}>
      {!readOnly && (
        <>
          <PageTitle
            icon={moduleIcon(path)}
            action={
              <button
                type="button"
                onClick={() => {
                  setNotice(null);
                  setDialog({ kind: "form", item: null });
                }}
                className={primaryButton}
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
                Nuevo gasto futuro
              </button>
            }
          >
            {title}
          </PageTitle>
          <p className="mt-1 text-sm text-muted">
            Registra lo que quieres ir juntando (un seguro, un viaje, una compra grande) con su monto y fecha. Cada ahorro que agregas aquí queda ligado a ese gasto, y te
            sugerimos cuánto apartar en cada ciclo de pago para llegar a tiempo.
          </p>
        </>
      )}

      <div className={`${readOnly ? "" : "mt-6 "}space-y-6 rounded-xl border border-border bg-surface p-4 sm:p-6`}>

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
                {readOnly
                  ? "Ahorro en la categoría Gastos futuros que aún no está asignado a ningún gasto."
                  : "Ahorro en la categoría Gastos futuros que aún no está asignado a ningún gasto. No se reparte solo: tú decides cuánto asignar a cada uno."}
              </p>
            </section>

            {deepLinkEdit && !readOnly && <EditDeepLink items={list.data.active} open={editing} onOpen={(item) => setDialog({ kind: "form", item })} />}

            {notice && (
              <p role="status" className="flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                <span className="min-w-0 break-words">{notice}</span>
              </p>
            )}

            <div className="space-y-3">
              <h3 className="text-base font-semibold tracking-tight">Por juntar</h3>
              {list.data.active.length === 0 ? (
                <EmptyNote icon={Target}>
                  {readOnly ? "Aún no hay gastos futuros registrados." : "Aún no tienes gastos futuros. Agrega el primero con el botón de arriba."}
                </EmptyNote>
              ) : (
                <>
                  <ActiveList
                    items={list.data.active}
                    onOpen={(item) => {
                      setNotice(null);
                      setDialog({ kind: "detail", item });
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
                  onOpen={(item) => {
                    setNotice(null);
                    setDialog({ kind: "paid", item });
                  }}
                />
              </div>
            )}
          </>
        )}

        {dialog?.kind === "detail" && (
          <ItemDetailDialog
            item={dialog.item}
            freeBalance={list.data?.free_balance ?? "0"}
            onClose={() => setDialog(null)}
            onSaving={(item) => setDialog({ kind: "amount", mode: "saving", item })}
            onAssign={(item) => setDialog({ kind: "amount", mode: "assign", item })}
            onPay={(item) => setDialog({ kind: "pay", item })}
            onEdit={(item) => setDialog({ kind: "form", item })}
            onDeleted={(item) => {
              setDialog(null);
              setNotice(`${item.name} se eliminó. Su ahorro volvió al saldo libre.`);
            }}
          />
        )}
        {dialog?.kind === "paid" && (
          <PaidDetailDialog
            item={dialog.item}
            onClose={() => setDialog(null)}
            onDeleted={(item) => {
              setDialog(null);
              setNotice(`${item.name} se quitó del historial. El gasto que registró se conserva.`);
            }}
          />
        )}
        {dialog?.kind === "form" && (
          <Modal title={dialog.item ? `Editar ${dialog.item.name}` : "Nuevo gasto futuro"} onClose={() => setDialog(null)} highlight={dialog.item !== null}>
            <ItemForm
              key={dialog.item?.id ?? "new"}
              editing={dialog.item}
              onSaved={(item, wasEdit) => {
                setDialog(null);
                setNotice(wasEdit ? `Cambios de ${item.name} guardados.` : `${item.name} agregado. Aparta ${formatMoney(item.suggested_monthly)} al mes para llegar a tiempo.`);
              }}
              onCancel={() => setDialog(null)}
            />
          </Modal>
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
        {dialog?.kind === "pay" && <PayDialog item={dialog.item} categories={categories} categoryRequired={categoryRequired} onClose={() => setDialog(null)} onPaid={onPaid} />}
      </div>
    </section>
  );
}
