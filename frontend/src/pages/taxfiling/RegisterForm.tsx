import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Save } from "lucide-react";
import { registerTaxFiling, type FilingResult, type RegisterInput, type TaxPreview } from "../../api/taxFiling";
import { TextField } from "../../components/AuthCard";
import { formatMoney, todayISO } from "../expenses/money";
import { Checkbox, ErrorBanner, primaryButton } from "../settings/ui";
import { describeTaxFilingError } from "./errors";
import { PaymentFields } from "./PaymentFields";
import { defaultPaymentDraft, toPaymentInput, validatePayment, type PaymentDraft, type PaymentErrors } from "./payment";
import { useInvalidateAfterFiling } from "./useInvalidate";

const MAX_FOLIO = 64;

interface Props {
  preview: TaxPreview;
  onRegistered: (result: FilingResult) => void;
}

/**
 * Registers the filing the user made in the SAT portal: the figures come from
 * the computed declaration, the user adds the filing date, the folio of the
 * acuse and, optionally, the payment.
 */
export function RegisterForm({ preview, onRegistered }: Props) {
  const invalidate = useInvalidateAfterFiling();
  // null = the user never touched the date, so it is "today" whenever it is read.
  const [date, setDate] = useState<string | null>(null);
  const [folio, setFolio] = useState("");
  const [paid, setPaid] = useState(false);
  const [payment, setPayment] = useState<PaymentDraft>(() => defaultPaymentDraft(preview.isr_due, preview.iva_due, todayISO()));
  const [errors, setErrors] = useState<{ date?: string; folio?: string; payment?: PaymentErrors }>({});
  const folioRef = useRef<HTMLInputElement>(null);
  const isrRef = useRef<HTMLInputElement>(null);
  const ivaRef = useRef<HTMLInputElement>(null);

  const register = useMutation({
    mutationFn: (input: RegisterInput) => registerTaxFiling(input),
    onSuccess: async (result) => {
      await invalidate();
      onRegistered(result);
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
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.folio) folioRef.current?.focus();
      else if (found.payment?.isr) isrRef.current?.focus();
      else if (found.payment?.iva) ivaRef.current?.focus();
      return;
    }
    const input: RegisterInput = { period: preview.period, filing_date: filingDate };
    if (folio.trim()) input.folio = folio.trim();
    if (!/^0+(\.0+)?$/.test(preview.iva_acreditable)) input.iva_acreditable = preview.iva_acreditable;
    if (paid) input.payment = toPaymentInput(payment);
    register.mutate(input);
  }

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="register-title" className="space-y-5 rounded-lg border border-border p-4">
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

      <div className="space-y-3">
        <Checkbox label="Ya pagué esta declaración al SAT" checked={paid} onChange={setPaid} />
        {paid ? (
          <PaymentFields draft={payment} onChange={setPayment} errors={errors.payment ?? {}} isrRef={isrRef} ivaRef={ivaRef} />
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
