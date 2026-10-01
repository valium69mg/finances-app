import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, XCircle } from "lucide-react";
import { rejectExpenseRequest, type ExpenseRequest } from "../../api/expenseRequests";
import { FieldError } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeRequestError } from "./errors";
import { MAX_COMMENT_LENGTH, validateComment } from "./form";
import { useInvalidateRequests } from "./useInvalidate";

interface Props {
  request: ExpenseRequest;
  onClose: () => void;
  onRejected: (request: ExpenseRequest) => void;
}

/** Rejection: the comment is required because it is what the requester reads. */
export function RejectDialog({ request, onClose, onRejected }: Props) {
  const invalidate = useInvalidateRequests();
  const [comment, setComment] = useState("");
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<HTMLTextAreaElement>(null);
  const id = useId();
  const errorId = `${id}-error`;
  // The modal focuses its first control on open; the comment is what this dialog is for.
  useEffect(() => ref.current?.focus(), []);

  const reject = useMutation({
    mutationFn: () => rejectExpenseRequest(request.id, comment.trim()),
    onSuccess: async (r) => {
      await invalidate();
      onRejected(r);
    },
    // A request decided meanwhile (409) or gone: show the real state behind the dialog's message.
    onError: () => void invalidate(),
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    reject.reset();
    const found = validateComment(comment);
    setError(found);
    if (found) {
      ref.current?.focus();
      return;
    }
    reject.mutate();
  }

  return (
    <Modal title="Rechazar petición" onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm text-muted">
          {request.description} · {formatMoney(request.amount)}. Quien la pidió recibirá tu comentario por correo.
        </p>
        <div>
          <label htmlFor={id} className="mb-1.5 block text-sm font-medium">
            Comentario
          </label>
          <textarea
            id={id}
            ref={ref}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            rows={4}
            maxLength={MAX_COMMENT_LENGTH + 50}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
            className="focus-ring block min-h-24 w-full rounded-lg border border-border bg-surface px-3 py-2 text-foreground transition-colors duration-200 aria-[invalid=true]:border-destructive"
          />
          {error && <FieldError id={errorId}>{error}</FieldError>}
        </div>
        {reject.isError && <ErrorBanner>{describeRequestError(reject.error)}</ErrorBanner>}
        <div className="flex flex-wrap gap-3">
          <button type="submit" disabled={reject.isPending} aria-busy={reject.isPending} className={dangerButton}>
            {reject.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <XCircle className="h-4 w-4" aria-hidden="true" />}
            {reject.isPending ? "Rechazando…" : "Rechazar petición"}
          </button>
          <button type="button" onClick={onClose} disabled={reject.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
