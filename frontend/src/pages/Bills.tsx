import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Loader2 } from "lucide-react";
import type { Bill, PayResult } from "../api/bills";
import { getSettings, settingsKeys } from "../api/settings";
import { InlineFormPanel } from "../components/InlineFormPanel";
import { BillForm } from "./bills/BillForm";
import { BillList } from "./bills/BillList";
import { describeBillsError } from "./bills/errors";
import { expenseCategories } from "./bills/form";
import { PayDialog } from "./bills/PayDialog";
import { formatMoney } from "./expenses/money";
import { dateLabel } from "./taxfiling/labels";
import { Checkbox, ErrorBanner, secondaryButton } from "./settings/ui";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";

/** Pagos recurrentes: bills and subscriptions with their next due date, paid as expenses or skipped. */
export function Bills() {
  const [editing, setEditing] = useState<Bill | null>(null);
  // The create form stays collapsed until asked for; editing a row opens it by itself.
  const [formOpen, setFormOpen] = useState(false);
  const [paying, setPaying] = useState<Bill | null>(null);
  const [includeInactive, setIncludeInactive] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  const categories = expenseCategories(settings.data);

  function onPaid(result: PayResult) {
    const { bill, expense } = result;
    setPaying(null);
    setEditing((cur) => (cur?.id === bill.id ? null : cur)); // its due date moved: drop the stale edit
    setNotice(
      `Pago de ${bill.name} registrado: ${formatMoney(expense.amount)} ${expense.currency} en ${expense.category}. Próximo vencimiento: ${dateLabel(bill.next_due_date)}.`,
    );
  }

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/pagos-recurrentes")}>Pagos recurrentes</PageTitle>
      <p className="mt-1 text-sm text-muted">
        Administra tus servicios y suscripciones: mira cuándo vence cada uno, regístralos como gasto al pagarlos u omite un vencimiento.
      </p>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {settings.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando configuración…
          </p>
        )}
        {settings.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeBillsError(settings.error)}</ErrorBanner>
            <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {settings.data && (
          <InlineFormPanel id="bill-form-panel" label="+ Nuevo pago recurrente" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={editing !== null}>
            <BillForm
              key={editing?.id ?? "new"}
              categories={categories}
              editing={editing}
              onSaved={(bill, wasEdit) => {
                setEditing(null);
                setFormOpen(false);
                setNotice(wasEdit ? `Cambios de ${bill.name} guardados.` : `${bill.name} agregado. Próximo vencimiento: ${dateLabel(bill.next_due_date)}.`);
              }}
              onCancelEdit={() => {
                setEditing(null);
                setFormOpen(false);
              }}
            />
          </InlineFormPanel>
        )}
      </div>

      {notice && (
        <p role="status" className="mt-6 flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span className="min-w-0 break-words">{notice}</span>
        </p>
      )}

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Próximos vencimientos</h2>
          <Checkbox label="Mostrar desactivados" checked={includeInactive} onChange={setIncludeInactive} />
        </div>
        <BillList
          includeInactive={includeInactive}
          editingId={editing?.id ?? null}
          onPay={(b) => {
            setNotice(null);
            setPaying(b);
          }}
          onEdit={(b) => {
            setNotice(null);
            setEditing(b);
          }}
          onNotice={setNotice}
          onChanged={(id) => setEditing((cur) => (cur?.id === id ? null : cur))}
        />
      </div>

      {paying && <PayDialog bill={paying} categories={categories} onClose={() => setPaying(null)} onPaid={onPaid} />}
    </section>
  );
}
