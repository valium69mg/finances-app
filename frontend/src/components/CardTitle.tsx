import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Tone } from "../lib/tones";
import { Link } from "react-router-dom";
import { IconChip } from "./IconChip";
import { Chevron, ROW_LINK } from "./LinkCard";

/**
 * Heading of a dashboard card: icon chip and title. With `to` the title is a real link stretched over the whole card
 * (the card needs `cardClass(to)`), with the chevron at the end.
 */
export function CardTitle({ id, icon, tone = "primary", to, children }: { id: string; icon: LucideIcon; tone?: Tone; to?: string; children: ReactNode }) {
  return (
    <h3 id={id} className="flex items-center gap-2 font-semibold">
      <IconChip icon={icon} tone={tone} size="sm" />
      <span className="min-w-0 break-words">
        {to ? (
          <Link to={to} className={`${ROW_LINK} after:rounded-lg`}>
            {children}
          </Link>
        ) : (
          children
        )}
      </span>
      {to && <Chevron className="ml-auto" />}
    </h3>
  );
}
