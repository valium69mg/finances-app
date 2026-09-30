import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import type { Expense, SaveExpenseResult } from "../api/expenses";
import { getSettings, settingsKeys } from "../api/settings";
import { TextField } from "../components/AuthCard";
import { BudgetFeedbackNote } from "./expenses/BudgetFeedbackNote";
import { ExpenseForm } from "./expenses/ExpenseForm";
import { ExpenseList } from "./expenses/ExpenseList";
import { describeExpenseError } from "./expenses/errors";
import { todayISO } from "./expenses/money";
import { ErrorBanner, secondaryButton } from "./settings/ui";

export function Expenses() {
  const [month, setMonth] = useState(() => todayISO().slice(0, 7));
  const [editing, setEditing] = useState<Expense | null>(null);
  const [feedback, setFeedback] = useState<SaveExpenseResult | null>(null);
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        Gastos
      </h1>
      <p className="mt-1 text-sm text-muted">Registra tus gastos y revisa cuánto llevas contra tu presupuesto.</p>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {settings.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando configuración…
          </p>
        )}
        {settings.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeExpenseError(settings.error)}</ErrorBanner>
            <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {settings.data && (
          <div className="space-y-4">
            <ExpenseForm
              key={editing?.id ?? "new"}
              settings={settings.data}
              editing={editing}
              onSaved={(result) => {
                setFeedback(result);
                setEditing(null);
              }}
              onCancelEdit={() => setEditing(null)}
            />
            {feedback && <BudgetFeedbackNote result={feedback} />}
          </div>
        )}
      </div>

      <div className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-lg font-semibold tracking-tight">Gastos del mes</h2>
          <div className="w-full sm:w-56">
            <TextField label="Mes" type="month" value={month} onChange={(e) => e.target.value && setMonth(e.target.value)} />
          </div>
        </div>
        <ExpenseList
          month={month}
          editingId={editing?.id ?? null}
          onEdit={(e) => {
            setFeedback(null);
            setEditing(e);
            window.scrollTo({ top: 0, behavior: "smooth" });
          }}
          onDeleted={(id) => setEditing((cur) => (cur?.id === id ? null : cur))}
        />
      </div>
    </section>
  );
}
