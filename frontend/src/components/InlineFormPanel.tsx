import { Plus } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";
import { secondaryButton } from "../pages/settings/ui";

const FIRST_FIELD = "input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled])";

interface Props {
  /** Id of the panel, referenced by the toggle's aria-controls. */
  id: string;
  /** Text of the toggle, e.g. "Nuevo ingreso". A leading "+ " is dropped: the Plus icon replaces it. */
  label: string;
  /** The create form is open. */
  open: boolean;
  onToggle: () => void;
  /** A row is being edited: the panel is forced open and the toggle is hidden. */
  editing: boolean;
  /** The form. Rendered only while the panel is expanded, so it mounts fresh on every open. */
  children: ReactNode;
}

/**
 * A create form collapsed behind a button that expands it inline (no modal). Expanded means `open` or
 * `editing`: starting to edit a row opens it on its own. The toggle exposes aria-expanded/aria-controls,
 * focus moves to the first field when it opens for a new record, and returns to the toggle when it closes.
 * The edit cue (scroll, focus and green highlight) stays with the form itself (useFocusOnEdit).
 */
export function InlineFormPanel({ id, label, open, onToggle, editing, children }: Props) {
  const expanded = open || editing;
  const toggleRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const wasExpanded = useRef(expanded);

  useEffect(() => {
    if (expanded && !editing) panelRef.current?.querySelector<HTMLElement>(FIRST_FIELD)?.focus();
    // Closing hands the focus back to the toggle instead of dropping it on the page.
    if (!expanded && wasExpanded.current) toggleRef.current?.focus();
    wasExpanded.current = expanded;
  }, [expanded, editing]);

  return (
    <div>
      {!editing && (
        <button ref={toggleRef} type="button" onClick={onToggle} aria-expanded={expanded} aria-controls={id} className={secondaryButton}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          {label.replace(/^\+\s*/, "")}
        </button>
      )}
      <div id={id} ref={panelRef} hidden={!expanded} className={expanded && !editing ? "mt-5" : undefined}>
        {expanded && children}
      </div>
    </div>
  );
}
