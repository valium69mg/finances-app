import { useId } from "react";
import { Coins } from "lucide-react";
import type { MovementKind, RecentMovement } from "../../api/dashboard";
import { CardTitle } from "../../components/CardTitle";
import { EmptyNote } from "../../components/EmptyNote";
import { KIND, TONES } from "../../lib/tones";
import { formatMoney } from "../expenses/money";
import { dateLabel } from "../taxfiling/labels";

/** Icon, color and the kind written out: the color never carries the meaning alone. */
function KindBadge({ kind }: { kind: MovementKind }) {
  const { icon: Icon, tone } = KIND[kind];
  return (
    <span className={`inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ${TONES[tone].chip}`}>
      <Icon className="h-4 w-4" aria-hidden="true" />
      {kind}
    </span>
  );
}

/** One mixed list of the latest movements across Ingreso, Gasto and Ahorro. */
export function RecentMovements({ movements }: { movements: RecentMovement[] }) {
  const titleId = useId();
  return (
    <section aria-labelledby={titleId} className="rounded-lg border border-border p-4">
      <CardTitle id={titleId} icon={Coins}>
        Últimos movimientos
      </CardTitle>
      {movements.length === 0 ? (
        <EmptyNote icon={Coins}>Aún no hay movimientos registrados.</EmptyNote>
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
