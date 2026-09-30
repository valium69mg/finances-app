import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Send } from "lucide-react";
import { issueInvoice, type InvoiceDetail } from "../../api/invoices";
import { TextField } from "../../components/AuthCard";
import { ErrorBanner, primaryButton } from "../settings/ui";
import { describeInvoiceError } from "./errors";
import { FileField } from "./FileField";
import { isValidUuid, validateFile } from "./files";
import { useInvalidateAfterInvoiceChange } from "./useInvalidate";

interface Props {
  invoiceId: number;
  onIssued: (detail: InvoiceDetail) => void;
}

type Errors = { xml?: string; pdf?: string; uuid?: string; form?: string };

/**
 * Marks a prepared invoice as issued once it was stamped in the SAT portal: the
 * user uploads the CFDI XML (the UUID and total are read from it) with an
 * optional PDF, or types the UUID by hand when there is no XML.
 */
export function IssuePanel({ invoiceId, onIssued }: Props) {
  const invalidate = useInvalidateAfterInvoiceChange();
  const [xml, setXml] = useState<File | null>(null);
  const [pdf, setPdf] = useState<File | null>(null);
  const [uuid, setUuid] = useState("");
  const [errors, setErrors] = useState<Errors>({});
  const xmlRef = useRef<HTMLInputElement>(null);
  const uuidRef = useRef<HTMLInputElement>(null);
  const pdfRef = useRef<HTMLInputElement>(null);

  const issue = useMutation({
    mutationFn: () => issueInvoice(invoiceId, { xml, pdf, uuid: xml ? undefined : uuid.trim() }),
    onSuccess: (detail) => {
      void invalidate();
      onIssued(detail);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: Errors = {};
    if (xml) {
      const problem = validateFile(xml, "xml");
      if (problem) found.xml = problem;
    } else if (uuid.trim() === "") {
      found.form = "Sube el XML del CFDI o escribe el UUID para marcarla como emitida.";
    } else if (!isValidUuid(uuid)) {
      found.uuid = "El UUID debe tener el formato 8-4-4-4-12 de caracteres hexadecimales, por ejemplo 6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B.";
    }
    if (pdf) {
      const problem = validateFile(pdf, "pdf");
      if (problem) found.pdf = problem;
    }
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.xml) xmlRef.current?.focus();
      else if (found.uuid || found.form) uuidRef.current?.focus();
      else if (found.pdf) pdfRef.current?.focus();
      return;
    }
    issue.mutate();
  }

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby={`issue-title-${invoiceId}`} className="space-y-4 rounded-lg border border-border p-4">
      <div>
        <h3 id={`issue-title-${invoiceId}`} className="text-base font-semibold tracking-tight">
          Marcar como emitida
        </h3>
        <p className="mt-1 text-sm text-muted">
          Cuando el SAT timbre la factura, sube el XML del CFDI: de ahí se toman el UUID y el total. El PDF es opcional.
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <FileField label="XML del CFDI" accept=".xml,application/xml,text/xml" onChange={setXml} error={errors.xml} hint="Hasta 1 MB." inputRef={xmlRef} />
        <FileField label="PDF (opcional)" accept=".pdf,application/pdf" onChange={setPdf} error={errors.pdf} hint="Hasta 10 MB." inputRef={pdfRef} />
      </div>
      <TextField
        label="UUID manual"
        hint={xml ? "Se tomará del XML." : "Solo si no tienes el XML."}
        value={uuid}
        onChange={(e) => setUuid(e.target.value)}
        disabled={xml !== null}
        error={errors.uuid ?? errors.form}
        inputRef={uuidRef}
        autoComplete="off"
        spellCheck={false}
      />
      {issue.isError && <ErrorBanner>{describeInvoiceError(issue.error)}</ErrorBanner>}
      <button type="submit" disabled={issue.isPending} aria-busy={issue.isPending} className={primaryButton}>
        {issue.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Send className="h-4 w-4" aria-hidden="true" />}
        {issue.isPending ? "Guardando…" : "Marcar como emitida"}
      </button>
    </form>
  );
}
