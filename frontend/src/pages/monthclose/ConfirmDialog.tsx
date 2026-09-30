import type { ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { Modal } from "../../components/Modal";
import { ErrorBanner, dangerButton, primaryButton, secondaryButton } from "../settings/ui";

interface Props {
  title: string;
  /** What is about to happen and what it means, in the dialog body. */
  children: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  /** The confirm button is styled as destructive (discarding data). */
  destructive?: boolean;
  pending: boolean;
  /** Spanish copy of the failure of the last attempt, if any; the dialog stays open. */
  error?: string | null;
  onConfirm: () => void;
  onCancel: () => void;
}

/** Confirmation dialog for the two irreversible-ish month close actions: storing a snapshot and discarding one. */
export function ConfirmDialog({ title, children, confirmLabel, cancelLabel, destructive = false, pending, error, onConfirm, onCancel }: Props) {
  return (
    <Modal title={title} onClose={pending ? () => {} : onCancel}>
      <div className="space-y-4">
        <div className="space-y-2 text-sm">{children}</div>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        <div className="flex flex-wrap gap-2">
          <button type="button" disabled={pending} onClick={onConfirm} className={destructive ? dangerButton : primaryButton}>
            {pending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
            {confirmLabel}
          </button>
          <button type="button" disabled={pending} onClick={onCancel} className={secondaryButton}>
            {cancelLabel}
          </button>
        </div>
      </div>
    </Modal>
  );
}
