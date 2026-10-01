import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Tone } from "../lib/tones";
import { IconChip } from "./IconChip";

/** Heading of a dashboard card: icon chip and title. */
export function CardTitle({ id, icon, tone = "primary", children }: { id: string; icon: LucideIcon; tone?: Tone; children: ReactNode }) {
  return (
    <h3 id={id} className="flex items-center gap-2 font-semibold">
      <IconChip icon={icon} tone={tone} size="sm" />
      <span className="min-w-0 break-words">{children}</span>
    </h3>
  );
}
