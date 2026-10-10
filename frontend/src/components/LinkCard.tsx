import type { LucideIcon } from "lucide-react";
import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { IconChip } from "./IconChip";
import { TONES, type Tone } from "../lib/tones";

/**
 * Link affordance of the interactive Panel, shared by every card and row that can be clicked:
 *  - `StatCard` is a summary figure; with `to` the whole card is a real link (keyboard focusable, visible focus ring,
 *    hover tint, chevron), without it the card looks static (no hover, no chevron, no pointer).
 *  - `ROW_LINKED`, `ROW_LINK` and `Chevron` make one row clickable by stretching the row's own link over it, so a table
 *    row or a list item stays semantic while the whole row is the target.
 * The link text is always the visible label; the chevron is decorative.
 */

/** Decorative chevron that tells a card or row is a link; it nudges on hover. */
export function Chevron({ className = "" }: { className?: string }) {
  return (
    <ChevronRight
      aria-hidden="true"
      className={`h-4 w-4 shrink-0 text-muted transition-transform duration-200 group-hover:translate-x-0.5 group-hover:text-foreground motion-reduce:transition-none ${className}`}
    />
  );
}

/**
 * Classes of the link inside a linked row or card: an invisible layer stretched over the whole container (which needs
 * `ROW_LINKED`), so the entire box is the target. Keyboard focus draws a ring on that layer. Put the visible label
 * inside the link and add a corner radius (`after:rounded-lg`) when the container has one.
 */
export const ROW_LINK = "focus-visible:outline-none after:absolute after:inset-0 after:content-[''] focus-visible:after:ring-2 focus-visible:after:ring-inset focus-visible:after:ring-ring";

/** Classes of a container (li, tr, div) that holds a `ROW_LINK`: hover tint and the chevron nudge. */
export const ROW_LINKED = "group relative transition-colors duration-200 hover:bg-primary/5";

interface StatCardProps {
  label: string;
  hint?: string;
  negative?: boolean;
  icon: LucideIcon;
  tone: Tone;
  /** Route the card links to. Without it the card is informative and not interactive. */
  to?: string;
  children: ReactNode;
}

/** A summary figure: label, amount and optional hint, linking to the page that explains it when `to` is given. */
export function StatCard({ label, hint, negative, icon, tone, to, children }: StatCardProps) {
  const box = `flex min-w-0 items-start gap-3 rounded-lg border p-4 ${TONES[tone].card}`;
  return (
    <div className={to ? `${box} group relative transition duration-200 hover:border-foreground/30 hover:shadow-sm` : box}>
      <IconChip icon={icon} tone={tone} onSoft />
      <div className="min-w-0 flex-1">
        <dt className="text-sm text-muted">
          {to ? (
            <Link to={to} className={`${ROW_LINK} after:rounded-lg`}>
              {label}
            </Link>
          ) : (
            label
          )}
        </dt>
        <dd className={`mt-1 break-words text-xl font-semibold tabular-nums ${negative ? "text-destructive" : ""}`}>{children}</dd>
        {hint && <p className="mt-1 text-xs text-muted">{hint}</p>}
      </div>
      {to && <Chevron className="mt-1" />}
    </div>
  );
}
