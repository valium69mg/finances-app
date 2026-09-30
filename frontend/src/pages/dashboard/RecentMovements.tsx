import { useId } from "react";
import type { MovementKind, RecentMovement } from "../../api/dashboard";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

const KIND_STYLE: Record<MovementKind, string> = {
  Ingreso: "border-accent/60 bg-accent/15 text-foreground",
  Gasto: "border-destructive/40 bg-destructive/10 text-destructive",
  Ahorro: "border-border bg-border/40 text-foreground",
};

/** The kind is always written out; the tint only reinforces it. */
function KindBadge({ kind }: { kind: MovementKind }) {
  return <span className={`inline-block shrink-0 whitespace-nowrap rounded-full border px-2 py-0.5 text-xs font-medium ${KIND_STYLE[kind]}`}>{kind}</span>;
}

/** One mixed list of the latest movements across Ingreso, Gasto and Ahorro. */
export function RecentMovements({ movements }: { movements: RecentMovement[] }) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <h3 id={titleId} className="font-semibold">
        Últimos movimientos
      </h3>
      {movements.length === 0 ? (
        <p className="mt-1 text-sm text-muted">Aún no hay movimientos registrados.</p>
      ) : (
        <ul className="mt-2 divide-y divide-border">
          {movements.map((m) => (
            <li key={m.id} className="flex items-start justify-between gap-3 py-2">
              <div className="min-w-0">
                <p className="break-words text-sm font-medium">{m.description || m.category}</p>
                <p className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted">
                  <KindBadge kind={m.kind} />
                  <span>{dateLabel(m.date)}</span>
                  {m.description && <span className="break-words">{m.category}</span>}
                </p>
              </div>
              <span className="shrink-0 text-sm font-medium tabular-nums">{formatMoney(m.amount_mxn)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
