import type { ReactNode } from "react";
import { clampPercent, formatPercent, LEVEL_LABEL, usageLevel, type UsageLevel } from "./format";

/** Fill color of the bar and text color of the level badge, by level (semantic tokens only). */
const FILL: Record<UsageLevel, string> = {
  normal: "bg-accent",
  warning: "bg-warning",
  danger: "bg-destructive",
};

const BADGE: Record<UsageLevel, string> = {
  normal: "border-accent/60 bg-accent/15 text-foreground",
  warning: "border-warning/60 bg-warning/15 text-foreground",
  danger: "border-destructive/50 bg-destructive/10 text-destructive",
};

interface Props {
  /** Stable id for the test hooks and the heading association. */
  id: string;
  title: string;
  /** Used percentage, 0..100. */
  percent: number;
  /** Accessible name of the bar, e.g. "Uso de CPU". */
  barLabel: string;
  children?: ReactNode;
}

/** A card with a labelled percentage bar colored by level, plus detail lines. */
export function UsageCard({ id, title, percent, barLabel, children }: Props) {
  const level = usageLevel(percent);
  const width = clampPercent(percent);
  return (
    <section aria-labelledby={`${id}-title`} data-testid={`${id}-card`} data-level={level} className="min-w-0 rounded-xl border border-border bg-surface p-4 sm:p-5">
      <div className="flex items-start justify-between gap-3">
        <h2 id={`${id}-title`} className="min-w-0 text-lg font-semibold tracking-tight">
          {title}
        </h2>
        <span className={`shrink-0 rounded-full border px-2.5 py-0.5 text-xs font-medium ${BADGE[level]}`}>{LEVEL_LABEL[level]}</span>
      </div>
      <p className="mt-3 text-3xl font-semibold tabular-nums">{formatPercent(percent)}</p>
      <div
        role="progressbar"
        aria-label={barLabel}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={width}
        aria-valuetext={formatPercent(percent)}
        className="mt-3 h-3 w-full overflow-hidden rounded-full bg-border/60"
      >
        <div className={`h-full rounded-full transition-[width] duration-300 motion-reduce:transition-none ${FILL[level]}`} style={{ width: `${width}%` }} />
      </div>
      <div className="mt-4 space-y-1 text-sm text-muted">{children}</div>
    </section>
  );
}

/** Placeholder with the same footprint while the first reading loads. */
export function UsageCardSkeleton() {
  return (
    <div aria-hidden="true" className="min-w-0 animate-pulse rounded-xl border border-border bg-surface p-4 motion-reduce:animate-none sm:p-5">
      <div className="h-6 w-1/2 rounded bg-border/60" />
      <div className="mt-4 h-9 w-1/3 rounded bg-border/60" />
      <div className="mt-3 h-3 w-full rounded-full bg-border/60" />
      <div className="mt-4 h-4 w-2/3 rounded bg-border/60" />
    </div>
  );
}
