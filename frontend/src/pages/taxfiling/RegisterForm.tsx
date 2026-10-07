import { useRef, useState, type FormEvent } from "react";
import { useFocusOnEdit } from "../../hooks/useFocusOnEdit";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Save } from "lucide-react";
import { registerTaxFiling, type FilingDocumentKind, type FilingResult, type RegisterInput, type TaxPreview } from "../../api/taxFiling";
import { TextField } from "../../components/AuthCard";
import { formatMoney, todayISO } from "../expenses/money";
import { FileField } from "../invoices/FileField";
import { Checkbox, ErrorBanner, primaryButton } from "../settings/ui";
import { DOCUMENT_ACCEPT, DOCUMENT_HINT, uploadDocuments, validateDocument, type UploadFailure } from "./documents";
import { describeTaxFilingError } from "./errors";
import { PaymentFields } from "./PaymentFields";
import { defaultPaymentDraft, toPaymentInput, validatePayment, type PaymentDraft, type PaymentErrors } from "./payment";
import { useInvalidateAfterFiling } from "./useInvalidate";

const MAX_FOLIO = 64;

interface Props {
  preview: TaxPreview;
  /** `failures` lists the files that could not be uploaded after the filing was saved. */
  onRegistered: (result: FilingResult, failures: UploadFailure[]) => void;
}

interface Registration {
  result: FilingResult;
  failures: UploadFailure[];
}

/**
 * Registers the filing the user made in the SAT portal: the figures come from
 * the computed declaration, the user adds the filing date, the folio of the
 * acuse and, optionally, the payment. The acuse file and the payment proof are
 * optional too: they are uploaded after the filing is saved, so a failed upload
 * never loses the declaration.
 */
export function RegisterForm({ preview, onRegistered }: Props) {
  const formRef = useFocusOnEdit<HTMLFormElement>(true, { focus: false, block: "nearest" });
  const invalidate = useInvalidateAfterFiling();
  // null = the user never touched the date, so it is "today" whenever it is read.
  const [date, setDate] = useState<string | null>(null);
  const [folio, setFolio] = useState("");
  const [paid, setPaid] = useState(false);
  const [payment, setPayment] = useState<PaymentDraft>(() => defaultPaymentDraft(preview.isr_due, preview.iva_due, todayISO()));
  const [acuse, setAcuse] = useState<File | null>(null);
  const [comprobante, setComprobante] = useState<File | null>(null);
  const [errors, setErrors] = useState<{ date?: string; folio?: string; payment?: PaymentErrors; acuse?: string; comprobante?: string }>({});
  const folioRef = useRef<HTMLInputElement>(null);
  const isrRef = useRef<HTMLInputElement>(null);
  const ivaRef = useRef<HTMLInputElement>(null);

  const acuseRef = useRef<HTMLInputElement>(null);
  const comprobanteRef = useRef<HTMLInputElement>(null);

  const register = useMutation({
    mutationFn: async (input: RegisterInput): Promise<Registration> => {
      const result = await registerTaxFiling(input);
      const files: { kind: FilingDocumentKind; file: File }[] = [];
      if (acuse) files.push({ kind: "acuse", file: acuse });
      if (paid && comprobante) files.push({ kind: "comprobante", file: comprobante });
      return { result, failures: await uploadDocuments(result.filing.period, files) };
    },
    onSuccess: async ({ result, failures }) => {
      await invalidate();
      onRegistered(result, failures);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    register.reset();
    const filingDate = date ?? todayISO();
    const found: typeof errors = {};
    if (!/^\d{4}-\d{2}-\d{2}$/.test(filingDate)) found.date = "Elige la fecha de presentación.";
    if (folio.trim().length > MAX_FOLIO) found.folio = `El folio puede tener hasta ${MAX_FOLIO} caracteres.`;
    if (paid) {
      const problems = validatePayment(payment);
      if (Object.keys(problems).length > 0) found.payment = problems;
    }
    if (acuse) {
      const problem = validateDocument(acuse, "acuse");
      if (problem) found.acuse = problem;
    }
    if (paid && comprobante) {
      const problem = validateDocument(comprobante, "comprobante");
      if (problem) found.comprobante = problem;
    }
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.folio) folioRef.current?.focus();
      else if (found.payment?.isr) isrRef.current?.focus();
      else if (found.payment?.iva) ivaRef.current?.focus();
      else if (found.acuse) acuseRef.current?.focus();
      else if (found.comprobante) comprobanteRef.current?.focus();
      return;
    }
    const input: RegisterInput = { period: preview.period, filing_date: filingDate };
    if (folio.trim()) input.folio = folio.trim();
    if (!/^0+(\.0+)?$/.test(preview.iva_acreditable)) input.iva_acreditable = preview.iva_acreditable;
    if (paid) input.payment = toPaymentInput(payment);
    register.mutate(input);
  }

  return (
    <form ref={formRef} noValidate onSubmit={onSubmit} aria-labelledby="register-title" className="space-y-5 rounded-lg border border-border p-4">
      <div>
        <h2 id="register-title" className="text-lg font-semibold tracking-tight">
          Registrar declaración presentada
        </h2>
        <p className="mt-1 text-sm text-muted">
          Presenta la declaración en el portal del SAT y registra aquí el resultado. Los importes son los del cálculo de arriba
          {!/^0+(\.0+)?$/.test(preview.iva_acreditable) && <>, con IVA acreditable de {formatMoney(preview.iva_acreditable)}</>}; no se pueden escribir a mano.
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField label="Fecha de presentación" type="date" value={date ?? todayISO()} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        <TextField
          label="Folio del acuse (opcional)"
          hint="Folio o número de operación del acuse del SAT."
          inputRef={folioRef}
          value={folio}
          onChange={(e) => setFolio(e.target.value)}
          error={errors.folio}
          autoComplete="off"
          spellCheck={false}
        />
      </div>
      <FileField
        label="Acuse (PDF)"
        accept={DOCUMENT_ACCEPT.acuse}
        hint={`Opcional. ${DOCUMENT_HINT.acuse} Puedes subirlo después desde Declaraciones presentadas.`}
        onChange={setAcuse}
        error={errors.acuse}
        inputRef={acuseRef}
      />

      <div className="space-y-3">
        <Checkbox
          label="Ya pagué esta declaración al SAT"
          checked={paid}
          onChange={(v) => {
            setPaid(v);
            if (!v) setComprobante(null);
          }}
        />
        {paid ? (
          <>
            <PaymentFields draft={payment} onChange={setPayment} errors={errors.payment ?? {}} isrRef={isrRef} ivaRef={ivaRef} />
            <FileField
              label="Comprobante de pago (imagen o PDF)"
              accept={DOCUMENT_ACCEPT.comprobante}
              hint={`Opcional. ${DOCUMENT_HINT.comprobante}`}
              onChange={setComprobante}
              error={errors.comprobante}
              inputRef={comprobanteRef}
            />
          </>
        ) : (
          <p className="text-sm text-muted">Si aún no pagas, se registra con el pago pendiente y lo marcas como pagado después en Declaraciones presentadas.</p>
        )}
      </div>

      {register.isError && <ErrorBanner>{describeTaxFilingError(register.error)}</ErrorBanner>}
      <button type="submit" disabled={register.isPending} aria-busy={register.isPending} className={primaryButton}>
        {register.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Save className="h-4 w-4" aria-hidden="true" />}
        {register.isPending ? "Registrando…" : "Registrar declaración"}
      </button>
    </form>
  );
}
