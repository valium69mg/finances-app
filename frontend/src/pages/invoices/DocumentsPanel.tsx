import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Download, FileText, Loader2, Upload } from "lucide-react";
import {
  attachInvoiceDocument,
  downloadInvoiceDocument,
  invoiceKeys,
  type DocumentKind,
  type InvoiceDocument,
  type InvoiceWarning,
} from "../../api/invoices";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { describeInvoiceError } from "./errors";
import { FileField } from "./FileField";
import { KIND_LABEL, formatBytes, validateFile } from "./files";

interface Props {
  invoiceId: number;
  documents: InvoiceDocument[];
  /** Files can only be attached or replaced while the invoice is issued. */
  canAttach: boolean;
  /** Warnings of an XML upload (total mismatch); the detail shows them. */
  onWarnings: (warnings: InvoiceWarning[]) => void;
}

/** Saves a downloaded blob through a temporary link; the object URL is released right after. */
function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

export function DocumentsPanel({ invoiceId, documents, canAttach, onWarnings }: Props) {
  const qc = useQueryClient();
  const [kind, setKind] = useState<DocumentKind>("pdf");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const [fileKey, setFileKey] = useState(0);
  const [downloading, setDownloading] = useState<number | null>(null);
  const [downloadError, setDownloadError] = useState<unknown>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const attach = useMutation({
    mutationFn: (f: File) => attachInvoiceDocument(invoiceId, kind, f),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: invoiceKeys.all });
      onWarnings(res.warnings);
      setFile(null);
      // Remounting the picker clears the chosen file.
      setFileKey((n) => n + 1);
    },
  });

  async function download(doc: InvoiceDocument) {
    setDownloadError(null);
    setDownloading(doc.id);
    try {
      saveBlob(await downloadInvoiceDocument(invoiceId, doc.id), doc.name);
    } catch (err) {
      setDownloadError(err);
    } finally {
      setDownloading(null);
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!file) {
      setFileError("Elige el archivo que quieres subir.");
      fileRef.current?.focus();
      return;
    }
    const problem = validateFile(file, kind);
    setFileError(problem);
    if (problem) {
      fileRef.current?.focus();
      return;
    }
    attach.mutate(file);
  }

  const has = (k: DocumentKind) => documents.some((d) => d.kind === k);

  return (
    <div className="space-y-4">
      <h3 className="text-base font-semibold tracking-tight">Documentos</h3>

      {documents.length === 0 ? (
        <p className="text-sm text-muted">Esta factura aún no tiene archivos guardados.</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {documents.map((d) => (
            <li key={d.id} className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0 flex-1">
                <p className="flex items-center gap-2 break-all font-medium">
                  <FileText className="h-4 w-4 shrink-0" aria-hidden="true" />
                  {d.name}
                </p>
                <p className="mt-0.5 text-sm text-muted">
                  {KIND_LABEL[d.kind]} · {formatBytes(d.size)} · subido el {d.uploaded_at.slice(0, 10)}
                </p>
              </div>
              <button
                type="button"
                onClick={() => void download(d)}
                disabled={downloading === d.id}
                aria-label={`Descargar ${d.name}`}
                className={`${secondaryButton} shrink-0`}
              >
                {downloading === d.id ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Download className="h-4 w-4" aria-hidden="true" />}
                Descargar
              </button>
            </li>
          ))}
        </ul>
      )}
      {downloadError !== null && <ErrorBanner>{describeInvoiceError(downloadError)}</ErrorBanner>}

      {canAttach && (
        <form noValidate onSubmit={onSubmit} aria-label="Adjuntar o reemplazar un documento" className="space-y-3 rounded-lg border border-border p-4">
          <p className="text-sm font-medium">Adjuntar o reemplazar un documento</p>
          <div className="grid gap-4 sm:grid-cols-2">
            <SelectField
              label="Tipo de documento"
              value={kind}
              onChange={(e) => {
                setKind(e.target.value as DocumentKind);
                setFileError(null);
              }}
              hint={has(kind) ? `Ya hay un ${KIND_LABEL[kind]}: el archivo nuevo lo reemplaza.` : undefined}
            >
              <option value="pdf">PDF</option>
              <option value="xml">XML del CFDI</option>
            </SelectField>
            <FileField
              key={fileKey}
              label="Archivo"
              accept={kind === "pdf" ? ".pdf,application/pdf" : ".xml,application/xml,text/xml"}
              onChange={(f) => {
                setFile(f);
                setFileError(null);
                attach.reset();
              }}
              error={fileError}
              hint={kind === "xml" ? "El UUID del XML debe ser el de la factura. Hasta 1 MB." : "Hasta 10 MB."}
              inputRef={fileRef}
            />
          </div>
          {attach.isError && <ErrorBanner>{describeInvoiceError(attach.error)}</ErrorBanner>}
          {attach.isSuccess && (
            <p role="status" className="text-sm">
              Documento guardado.
            </p>
          )}
          <button type="submit" disabled={attach.isPending} aria-busy={attach.isPending} className={primaryButton}>
            {attach.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Upload className="h-4 w-4" aria-hidden="true" />}
            {attach.isPending ? "Subiendo…" : "Subir documento"}
          </button>
        </form>
      )}
    </div>
  );
}
