import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Loader2, Save } from "lucide-react";
import { Modal } from "../../components/Modal";
import { ErrorBanner, describeSaveError, primaryButton, secondaryButton } from "./ui";

/** The slice of a mutation the dialog needs (independent of the mutation's variables type). */
interface SaveState {
  isPending: boolean;
  isError: boolean;
  error: unknown;
}

interface Props {
  title: string;
  onClose: () => void;
  save: SaveState;
  /** Validates and, when valid, starts the save. Returns false when the form has errors. */
  onSubmit: () => boolean;
  /** Highlight the body as an "edit here" cue (when editing something that already exists). */
  editing?: boolean;
  submitLabel?: string;
  /** Extra buttons next to Cancel (for example "Cancelar pausa"). */
  extraActions?: ReactNode;
  /** Blocking validation message shown right above the buttons (it must be visible where the user acts). */
  notice?: ReactNode;
  children: ReactNode;
}

/**
 * Modal with a form for editing one item or block of the settings: fields, a save
 * error from the server, Save and Cancel. The caller closes it when the save succeeds.
 */
export function EditDialog({ title, onClose, save, onSubmit, editing = true, submitLabel = "Guardar cambios", extraActions, notice, children }: Props) {
  const formRef = useRef<HTMLFormElement>(null);
  const [attempt, setAttempt] = useState(0);

  // Focus the first invalid field after a failed validation.
  useEffect(() => {
    if (attempt === 0) return;
    formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  }, [attempt]);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!onSubmit()) setAttempt((n) => n + 1);
  }

  return (
    <Modal title={title} onClose={onClose} highlight={editing}>
      <form ref={formRef} noValidate onSubmit={handleSubmit} className="space-y-5">
        {children}
        {notice}
        {save.isError && <ErrorBanner>{describeSaveError(save.error)}</ErrorBanner>}
        <div className="flex flex-wrap items-center gap-3">
          <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
            {save.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Save className="h-4 w-4" aria-hidden="true" />}
            {save.isPending ? "Guardando…" : submitLabel}
          </button>
          <button type="button" onClick={onClose} className={secondaryButton}>
            Cancelar
          </button>
          {extraActions}
        </div>
      </form>
    </Modal>
  );
}
