import { useState } from "react";
import { Pencil } from "lucide-react";
import type { Bracket } from "../../api/settings";
import { formatMoney, formatRatePercent } from "../expenses/money";
import { BracketsDialog } from "./BracketsDialog";
import { SectionHeader, secondaryButton } from "./ui";

/** Read-only table of the RESICO brackets; the set is edited together in a dialog. */
export function BracketsSection({ data }: { data: Bracket[] }) {
  const [editing, setEditing] = useState(false);
  return (
    <div className="space-y-6">
      <SectionHeader
        title="Rangos de RESICO"
        description="Límite superior mensual de ingresos (MXN) y tasa de ISR de cada rango, de menor a mayor."
        action={
          <button type="button" onClick={() => setEditing(true)} className={secondaryButton}>
            <Pencil className="h-4 w-4" aria-hidden="true" />
            Editar rangos
          </button>
        }
      />
      {data.length === 0 ? (
        <p className="text-sm text-muted">Aún no hay rangos configurados.</p>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-surface">
          <table className="w-full text-sm">
            <caption className="sr-only">Rangos de RESICO</caption>
            <thead>
              <tr className="border-b border-border text-left text-muted">
                <th scope="col" className="px-4 py-3 font-medium">
                  Hasta (MXN al mes)
                </th>
                <th scope="col" className="px-4 py-3 text-right font-medium">
                  Tasa de ISR
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {data.map((b, i) => (
                <tr key={i}>
                  <td className="px-4 py-3 tabular-nums">{formatMoney(b.upper)}</td>
                  <td className="px-4 py-3 text-right tabular-nums">{formatRatePercent(b.rate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && <BracketsDialog brackets={data} onClose={() => setEditing(false)} />}
    </div>
  );
}
