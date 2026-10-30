import { Loader2, Trash2 } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { dangerButton, secondaryButton } from "../pages/settings/ui";

interface Props {
  /** What is being deleted, for the accessible names ("Eliminar Laptop"). */
  name: string;
  /** The confirmation question, including what happens to related data. */
  question: ReactNode;
  pending: boolean;
  onConfirm: () => void;
  /** Called when the user starts the deletion (so the caller can clear a previous error). */
  onStart?: () => void;
  /** Label of the first button. */
  label?: string;
  confirmLabel?: string;
}

/**
 * "Eliminar" that asks before deleting: the button turns into the question with "Sí, eliminar" and "Cancelar".
 * The focus moves to Cancelar when the question appears (so Escape and Tab keep working inside a dialog) and goes
 * back to "Eliminar" when it is cancelled.
 */
export function DeleteAction({ name, question, pending, onConfirm, onStart, label = "Eliminar", confirmLabel = "Sí, eliminar" }: Props) {
  const [confirming, setConfirming] = useState(false);
  const startRef = useRef<HTMLButtonElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const wasConfirming = useRef(false);

  useEffect(() => {
    if (confirming) cancelRef.current?.focus();
    else if (wasConfirming.current) startRef.current?.focus();
    wasConfirming.current = confirming;
  }, [confirming]);

  if (!confirming) {
    return (
      <button
        ref={startRef}
        type="button"
        aria-label={`${label} ${name}`}
        onClick={() => {
          onStart?.();
          setConfirming(true);
        }}
        className={dangerButton}
      >
        <Trash2 className="h-4 w-4" aria-hidden="true" />
        {label}
      </button>
    );
  }
  return (
    <div role="group" aria-label={`Confirmar eliminación de ${name}`} className="flex flex-wrap items-center gap-2">
      <span className="text-sm">{question}</span>
      <button type="button" disabled={pending} onClick={onConfirm} className={dangerButton}>
        {pending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
        {confirmLabel}
      </button>
      <button ref={cancelRef} type="button" disabled={pending} onClick={() => setConfirming(false)} className={secondaryButton}>
        Cancelar
      </button>
    </div>
  );
}
