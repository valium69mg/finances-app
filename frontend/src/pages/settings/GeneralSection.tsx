import { useState, type ReactNode } from "react";
import { Pencil } from "lucide-react";
import type { General } from "../../api/settings";
import { baseIncome, formatPlain, rateToPercent } from "../../lib/planning";
import { formatMoney, formatRatePercent, todayISO } from "../expenses/money";
import { BLOCK_TITLES, GeneralDialog, SPLIT_LABELS, orderedSplitKeys } from "./GeneralDialog";
import { currentPeriodText, cycleStartLabel } from "./summary";
import { Card, SectionHeader, secondaryButton } from "./ui";
import type { GeneralBlock } from "./validation";

function Block({ id, block, onEdit, children }: { id: string; block: GeneralBlock; onEdit: (b: GeneralBlock) => void; children: ReactNode }) {
  const title = BLOCK_TITLES[block];
  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <h3 id={id} className="min-w-0 break-words font-semibold">
          {title}
        </h3>
        <button type="button" onClick={() => onEdit(block)} aria-label={`Editar ${title.toLowerCase()}`} className={`${secondaryButton} shrink-0`}>
          <Pencil className="h-4 w-4" aria-hidden="true" />
          Editar
        </button>
      </div>
      {children}
    </Card>
  );
}

function Facts({ items }: { items: [string, string][] }) {
  return (
    <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
      {items.map(([label, value]) => (
        <div key={label} className="min-w-0">
          <dt className="text-muted">{label}</dt>
          <dd className="break-words font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** General settings as summary cards; each block is edited on its own in a dialog. */
export function GeneralSection({ data }: { data: General }) {
  const [editing, setEditing] = useState<GeneralBlock | null>(null);
  const base = baseIncome(data.salary_usd, data.fx_rate_applied);
  const splitKeys = orderedSplitKeys(data.extra_income_split);

  return (
    <div className="space-y-6">
      <SectionHeader title="General" description="Ingresos, tipo de cambio y reglas base de tu presupuesto." />
      <div className="grid gap-4 xl:grid-cols-2">
        <Block id="general-income" block="income" onEdit={setEditing}>
          <p className="text-sm">
            {base ? (
              <>
                {formatPlain(data.salary_usd)} USD × {formatPlain(data.fx_rate_applied)} = <span className="font-semibold">{formatMoney(base)}</span> al mes
              </>
            ) : (
              <span className="text-muted">Aún no hay ingreso base: captura el salario y el tipo de cambio.</span>
            )}
          </p>
          <Facts items={[["Comisión de Morse", formatRatePercent(data.morse_fee_rate)]]} />
        </Block>

        <Block id="general-extra" block="extra" onEdit={setEditing}>
          <Facts
            items={[
              ["Ingreso extra estimado", formatMoney(data.extra_income_estimate_mxn)],
              ["Fondo de emergencia", `${formatPlain(data.emergency_months)} meses`],
              ["Presupuesto con ingreso extra", data.budget_includes_extra_income ? "Sí lo incluye" : "No lo incluye"],
            ]}
          />
        </Block>

        <Block id="general-period" block="period" onEdit={setEditing}>
          <Facts items={[["Inicio del periodo", cycleStartLabel(data.cycle_start_day)]]} />
          <p className="text-sm font-medium">{currentPeriodText(data.cycle_start_day, todayISO())}</p>
        </Block>

        {splitKeys.length > 0 && (
          <Block id="general-split" block="split" onEdit={setEditing}>
            <Facts items={splitKeys.map((k): [string, string] => [SPLIT_LABELS[k] ?? k, `${rateToPercent(data.extra_income_split[k])} %`])} />
          </Block>
        )}

        {data.investment_allocation.length > 0 && (
          <Block id="general-alloc" block="alloc" onEdit={setEditing}>
            <Facts items={data.investment_allocation.map((w): [string, string] => [w.key, `${rateToPercent(w.value)} %`])} />
          </Block>
        )}
      </div>
      {editing && <GeneralDialog data={data} block={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}
