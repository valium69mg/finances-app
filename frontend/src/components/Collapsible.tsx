import { ChevronDown } from "lucide-react";
import { useId, useState, type ReactNode } from "react";

interface Props {
  /** Text of the trigger. */
  title: ReactNode;
  /** Optional line under the title (a count, a total). */
  summary?: ReactNode;
  /** Start expanded. Only read on mount; use `open` + `onOpenChange` to control it. */
  defaultOpen?: boolean;
  /** Controlled state. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** Heading level of the trigger's wrapper (the button lives inside it). */
  as?: "h2" | "h3" | "h4";
  className?: string;
  children: ReactNode;
}

/**
 * A heading whose button shows or hides a content region. The button exposes aria-expanded/aria-controls and is a
 * native button, so Enter and Space work; the chevron rotates unless the user prefers reduced motion. The content is
 * not rendered while collapsed, so nothing hidden is reachable by Tab or read by screen readers.
 */
export function Collapsible({ title, summary, defaultOpen = false, open, onOpenChange, as: Heading = "h3", className = "", children }: Props) {
  const [inner, setInner] = useState(defaultOpen);
  const expanded = open ?? inner;
  const regionId = useId();
  const buttonId = useId();

  function toggle() {
    if (open === undefined) setInner(!expanded);
    onOpenChange?.(!expanded);
  }

  return (
    <div className={`rounded-xl border border-border ${className}`}>
      <Heading>
        <button
          type="button"
          id={buttonId}
          onClick={toggle}
          aria-expanded={expanded}
          aria-controls={regionId}
          className="focus-ring flex min-h-11 w-full items-center justify-between gap-3 rounded-xl px-4 py-2 text-left transition-colors duration-200 hover:bg-primary/5"
        >
          <span className="min-w-0">
            <span className="block break-words text-base font-semibold tracking-tight">{title}</span>
            {summary && <span className="block text-sm font-normal text-muted">{summary}</span>}
          </span>
          <ChevronDown
            className={`h-5 w-5 shrink-0 text-muted transition-transform duration-200 motion-reduce:transition-none ${expanded ? "rotate-180" : ""}`}
            aria-hidden="true"
          />
        </button>
      </Heading>
      <div id={regionId} role="region" aria-labelledby={buttonId} hidden={!expanded} className="border-t border-border p-4">
        {expanded && children}
      </div>
    </div>
  );
}
