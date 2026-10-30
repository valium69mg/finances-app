import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Tone } from "../lib/tones";
import { IconChip } from "./IconChip";

/**
 * Page heading (the page's h1) with an icon chip: the kind's tone on Ingresos, Gastos and Ahorros, primary elsewhere.
 * `action` puts the page's main buttons (Nuevo …) on the same line, wrapping under the title on a phone.
 */
export function PageTitle({ icon, tone = "primary", children, action }: { icon: LucideIcon; tone?: Tone; children: ReactNode; action?: ReactNode }) {
  const heading = (
    <h1 id="page-title" className="flex min-w-0 items-center gap-3 text-2xl font-semibold tracking-tight">
      <IconChip icon={icon} tone={tone} size="lg" />
      <span className="min-w-0 break-words">{children}</span>
    </h1>
  );
  if (!action) return heading;
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
      {heading}
      <div className="flex flex-wrap gap-2">{action}</div>
    </div>
  );
}
