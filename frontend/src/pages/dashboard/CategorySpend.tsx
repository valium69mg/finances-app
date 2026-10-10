import { useId } from "react";
import { Link } from "react-router-dom";
import { ArrowUpRight } from "lucide-react";
import type { PersonalCategorySpend } from "../../api/personal";
import { CardTitle } from "../../components/CardTitle";
import { EmptyNote } from "../../components/EmptyNote";
import { IconChip } from "../../components/IconChip";
import { Chevron, ROW_LINK, ROW_LINKED } from "../../components/LinkCard";
import { categoryIcon } from "../../lib/categoryIcon";
import { formatMoney, percentOf } from "../expenses/money";

/**
 * What each Gasto category spent in the cycle, with its share of the total spent. A row is a link when `linkFor` gives
 * it a target (her Panel sends it to Mis gastos filtered by that category).
 */
export function CategorySpend({
  categories,
  total,
  linkFor,
}: {
  categories: PersonalCategorySpend[];
  /** Total spent in the cycle: the base of every share. */
  total: string;
  linkFor?: (c: PersonalCategorySpend) => string | undefined;
}) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <CardTitle id={titleId} icon={ArrowUpRight} tone="expense">
        Gastado por categoría
      </CardTitle>
      {categories.length === 0 ? (
        <EmptyNote icon={ArrowUpRight}>Aún no hay gastos en este periodo.</EmptyNote>
      ) : (
        <ul aria-labelledby={titleId} className="mt-2 divide-y divide-border">
          {categories.map((c) => {
            const to = linkFor?.(c);
            const share = percentOf(c.spent, total);
            return (
              <li key={c.category_id} data-testid="category-spend" className={`flex items-center gap-3 px-1 py-3 ${to ? `${ROW_LINKED} rounded-lg` : ""}`}>
                <IconChip icon={categoryIcon(c.category)} tone="expense" size="sm" />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                    <span className="min-w-0 break-words font-medium">
                      {to ? (
                        <Link to={to} className={`${ROW_LINK} after:rounded-lg`}>
                          {c.category}
                        </Link>
                      ) : (
                        c.category
                      )}
                    </span>
                    <span className="shrink-0 text-sm tabular-nums">
                      {formatMoney(c.spent)} <span className="text-muted">· {Math.round(share)}%</span>
                    </span>
                  </div>
                  <div
                    role="progressbar"
                    aria-label={`Parte del gasto en ${c.category}`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={Math.round(share)}
                    className="mt-2 h-2 w-full overflow-hidden rounded-full bg-border"
                  >
                    <div className="h-full rounded-full bg-expense" style={{ width: `${share}%` }} />
                  </div>
                </div>
                {to && <Chevron />}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
