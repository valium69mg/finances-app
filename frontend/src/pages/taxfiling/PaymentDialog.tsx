import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Wallet } from "lucide-react";
import { payTaxFiling, type Filing, type FilingResult } from "../../api/taxFiling";
import { Modal } from "../../components/Modal";
import { formatMoney, todayISO } from "../expenses/money";
import { FileField } from "../invoices/FileField";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { DOCUMENT_ACCEPT, DOCUMENT_HINT, uploadDocuments, validateDocument, type UploadFailure } from "./documents";
import { describeTaxFilingError } from "./errors";
import { periodLabel } from "./labels";
import { PaymentFields } from "./PaymentFields";
import { amountToPay, defaultPaymentDraft, toPaymentInput, validatePayment, type PaymentErrors } from "./payment";
import { useInvalidateAfterFiling } from "./useInvalidate";

interface Props {
  filing: Filing;
  onClose: () => void;
  /** `failures` lists the files that could not be uploaded after the payment was saved. */
  onPaid: (result: FilingResult, failures: UploadFailure[]) => void;
}

interface Payment {
  result: FilingResult;
  failures: UploadFailure[];
}

/**
 * Records the payment of a filing that was registered with the payment
 * pending. Recording it as an Impuestos expense is optional and off by default,
 * and so is the payment proof file, which is uploaded once the payment is saved.
 */
export function PaymentDialog({ filing, onClose, onPaid }: Props) {
  const invalidate = useInvalidateAfterFiling();
  const [draft, setDraft] = useState(() => defaultPaymentDraft(filing.isr_due, filing.iva_due, todayISO()));
  const [errors, setErrors] = useState<PaymentErrors>({});
  const [proof, setProof] = useState<File | null>(null);
  const [proofError, setProofError] = useState<string | null>(null);
  const proofRef = useRef<HTMLInputElement>(null);
  const isrRef = useRef<HTMLInputElement>(null);
  const ivaRef = useRef<HTMLInputElement>(null);

  const pay = useMutation({
    mutationFn: async (): Promise<Payment> => {
      const result = await payTaxFiling(filing.period, toPaymentInput(draft));
      const failures = proof ? await uploadDocuments(filing.period, [{ kind: "comprobante", file: proof }]) : [];
      return { result, failures };
    },
    onSuccess: async ({ result, failures }) => {
      await invalidate();
      onPaid(result, failures);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    pay.reset();
    const found = validatePayment(draft);
    setErrors(found);
    const proofProblem = proof ? validateDocument(proof, "comprobante") : null;
    setProofError(proofProblem);
    if (Object.keys(found).length > 0 || proofProblem) {
      if (found.isr) isrRef.current?.focus();
      else if (found.iva) ivaRef.current?.focus();
      else proofRef.current?.focus();
      return;
    }
    pay.mutate();
  }

  return (
    <Modal title={`Registrar pago de ${periodLabel(filing.period)}`} onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm text-muted">
          Según la declaración, debías pagar {formatMoney(filing.isr_due)} de ISR y {formatMoney(amountToPay(filing.iva_due))} de IVA. Anota lo que
          pagaste realmente.
        </p>
        <PaymentFields draft={draft} onChange={setDraft} errors={errors} isrRef={isrRef} ivaRef={ivaRef} />
        <FileField
          label="Comprobante de pago (opcional)"
          accept={DOCUMENT_ACCEPT.comprobante}
          hint={DOCUMENT_HINT.comprobante}
          onChange={setProof}
          error={proofError}
          inputRef={proofRef}
        />
        {pay.isError && <ErrorBanner>{describeTaxFilingError(pay.error)}</ErrorBanner>}
        <div className="flex flex-wrap gap-3">
          <button type="submit" disabled={pay.isPending} aria-busy={pay.isPending} className={primaryButton}>
            {pay.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Wallet className="h-4 w-4" aria-hidden="true" />}
            {pay.isPending ? "Guardando…" : "Registrar pago"}
          </button>
          <button type="button" onClick={onClose} disabled={pay.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
