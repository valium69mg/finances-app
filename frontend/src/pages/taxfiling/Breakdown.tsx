import type { ReactNode } from "react";
import { formatMoney, formatRatePercent } from "../expenses/money";
import { dateLabel } from "./labels";

/** Figures shared by the computed declaration and a registered filing. */
export interface DeclarationAmounts {
  income_collected: string;
  isr_rate: string;
  isr_accrued: string;
  isr_withheld: string;
  isr_due: string;
  iva_transferred: string;
  iva_withheld: string;
  iva_acreditable: string;
  iva_due: string;
  total_to_pay: string;
  due_date: string;
  /** Income taxed at 0%; only the computed declaration reports it. */
  export_base?: string;
}

function Row({ label, children, strong = false }: { label: string; children: ReactNode; strong?: boolean }) {
  return (
    <div className={`flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 py-1.5 ${strong ? "font-semibold" : ""}`}>
      <dt className={strong ? "" : "text-muted"}>{label}</dt>
      <dd className="tabular-nums">{children}</dd>
    </div>
  );
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="min-w-0">
      <h3 className="mb-1 text-sm font-semibold tracking-tight">{title}</h3>
      <dl className="divide-y divide-border text-sm">{children}</dl>
    </section>
  );
}

/** The declaration as the SAT portal asks for it: income, ISR and IVA, and the amount to pay. */
export function Breakdown({ amounts }: { amounts: DeclarationAmounts }) {
  const ivaInFavor = amounts.iva_due.trim().startsWith("-");
  return (
    <div className="space-y-4">
      <div className="grid gap-x-8 gap-y-5 md:grid-cols-2">
        <Group title="Ingresos e ISR">
          <Row label="Ingresos cobrados (sin IVA)">{formatMoney(amounts.income_collected)}</Row>
          <Row label="Tasa RESICO aplicable">{formatRatePercent(amounts.isr_rate)}</Row>
          <Row label="ISR causado">{formatMoney(amounts.isr_accrued)}</Row>
          <Row label="ISR retenido por terceros">{formatMoney(amounts.isr_withheld)}</Row>
          <Row label="ISR a pagar" strong>
            {formatMoney(amounts.isr_due)}
          </Row>
        </Group>
        <Group title="IVA">
          <Row label="IVA trasladado">{formatMoney(amounts.iva_transferred)}</Row>
          <Row label="IVA retenido por terceros">{formatMoney(amounts.iva_withheld)}</Row>
          <Row label="IVA acreditable (gastos deducibles)">{formatMoney(amounts.iva_acreditable)}</Row>
          <Row label={ivaInFavor ? "IVA a favor (saldo a favor)" : "IVA a cargo"} strong>
            {formatMoney(ivaInFavor ? amounts.iva_due.trim().slice(1) : amounts.iva_due)}
          </Row>
          {amounts.export_base !== undefined && (
            <Row label="Actos a tasa 0% (exportación, Cliente USA)">{formatMoney(amounts.export_base)}</Row>
          )}
        </Group>
      </div>
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 rounded-lg border border-border bg-primary/5 px-4 py-3">
        <p className="font-semibold">Total a pagar al SAT</p>
        <p className="text-lg font-semibold tabular-nums">{formatMoney(amounts.total_to_pay)}</p>
      </div>
      <p className="text-sm text-muted">
        Fecha límite de presentación: <span className="font-medium text-foreground">{dateLabel(amounts.due_date)}</span>. Si cae en fin de
        semana o feriado, se recorre al siguiente día hábil. Valores estimados: verifica con tu contador antes de presentar.
      </p>
    </div>
  );
}
