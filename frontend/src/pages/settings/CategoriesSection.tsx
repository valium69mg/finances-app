import { useState } from "react";
import { Plus } from "lucide-react";
import type { Category, General } from "../../api/settings";
import { baseIncome, distribution, isBudgetKind } from "../../lib/planning";
import { formatMoney } from "../expenses/money";
import { CategoryDialog } from "./CategoryDialog";
import { DistributionBar } from "./DistributionBar";
import { budgetShare, groupCategories } from "./summary";
import { SectionHeader, rowButton, secondaryButton } from "./ui";

/** Categories grouped by kind, each a row with its budget and share of the base income; a click edits it in a dialog. */
export function CategoriesSection({ data, general }: { data: Category[]; general: General }) {
  /** Position of the category being edited, null for a new one; undefined when no dialog is open. */
  const [editing, setEditing] = useState<number | null | undefined>(undefined);

  // Base monthly income (estimate for planning); null disables every percentage feature.
  const base = baseIncome(general.salary_usd, general.fx_rate_applied);
  const dist = base ? distribution(data.filter((c) => isBudgetKind(c.kind)).map((c) => ({ kind: c.kind, budget: c.budget ?? "" })), base) : null;
  const groups = groupCategories(data);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Categorías y presupuestos"
        description="Presupuesto mensual por categoría. Toca una para editar su presupuesto, lo que incluye y sus palabras clave."
        action={
          <button type="button" onClick={() => setEditing(null)} className={secondaryButton}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Agregar categoría
          </button>
        }
      />
      {dist && base && (
        <>
          <DistributionBar distribution={dist} salaryUsd={general.salary_usd} fxRate={general.fx_rate_applied} />
          <p className="text-sm font-medium">
            Presupuestado: {formatMoney(dist.assigned)} de {formatMoney(dist.base)} ({dist.assignedPct.replace(/^-/, "")} %)
          </p>
        </>
      )}
      {data.length === 0 && <p className="text-sm text-muted">Aún no hay categorías configuradas.</p>}
      {groups.map((g) => (
        <section key={g.kind} aria-labelledby={`cat-group-${g.kind}`} className="space-y-2">
          <h3 id={`cat-group-${g.kind}`} className="text-sm font-semibold uppercase tracking-wide text-muted">
            {g.kind}
          </h3>
          <ul className="space-y-2">
            {g.items.map(({ category: c, index }) => {
              const share = isBudgetKind(c.kind) ? budgetShare(c.budget, base) : null;
              return (
                <li key={`${c.kind}-${c.name}`}>
                  <button type="button" onClick={() => setEditing(index)} className={`${rowButton} flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:gap-4`}>
                    <span className="min-w-0 flex-1 break-words font-medium">{c.name}</span>
                    <span className="flex shrink-0 items-center gap-3 text-sm sm:w-72 sm:justify-end">
                      <span className={c.budget ? "tabular-nums" : "text-muted"}>{c.budget ? formatMoney(c.budget) : "Sin presupuesto"}</span>
                      {share && <span className="w-14 text-right tabular-nums text-muted">{share.percent} %</span>}
                    </span>
                    {share && (
                      <span aria-hidden="true" className="h-1.5 w-full overflow-hidden rounded-full bg-border/50 sm:w-32">
                        <span className="block h-full rounded-full bg-primary" style={{ width: `${share.width}%` }} />
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
      {editing !== undefined && <CategoryDialog saved={data} index={editing} base={base} onClose={() => setEditing(undefined)} />}
    </div>
  );
}
