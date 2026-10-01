import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Tone } from "../lib/tones";
import { IconChip } from "./IconChip";

/** Page heading (the page's h1) with an icon chip: the kind's tone on Ingresos, Gastos and Ahorros, primary elsewhere. */
export function PageTitle({ icon, tone = "primary", children }: { icon: LucideIcon; tone?: Tone; children: ReactNode }) {
  return (
    <h1 id="page-title" className="flex items-center gap-3 text-2xl font-semibold tracking-tight">
      <IconChip icon={icon} tone={tone} size="lg" />
      <span className="min-w-0 break-words">{children}</span>
    </h1>
  );
}
