import { useRef, useState, type ChangeEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Download, FileText, Loader2, Upload } from "lucide-react";
import { attachTaxFilingDocument, downloadTaxFilingDocument, taxFilingKeys, type Filing, type FilingDocument, type FilingDocumentKind } from "../../api/taxFiling";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { formatBytes } from "../invoices/files";
import { DOCUMENT_ACCEPT, DOCUMENT_HINT, DOCUMENT_LABEL, DOCUMENT_NOUN, saveBlob, validateDocument } from "./documents";
import { describeDocumentError } from "./errors";

const KINDS: FilingDocumentKind[] = ["acuse", "comprobante"];

interface RowProps {
  period: string;
  kind: FilingDocumentKind;
  doc: FilingDocument | undefined;
}

/** One document slot: download and replace when there is a file, upload when there is none. */
function DocumentRow({ period, kind, doc }: RowProps) {
  const qc = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState<unknown>(null);
  const [downloading, setDownloading] = useState(false);
  const noun = DOCUMENT_NOUN[kind];

  const attach = useMutation({
    mutationFn: (file: File) => attachTaxFilingDocument(period, kind, file),
    onSuccess: () => qc.invalidateQueries({ queryKey: taxFilingKeys.all }),
  });

  function onPick(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    // Clearing the input lets the same file be picked again after a failure.
    e.target.value = "";
    if (!file) return;
    attach.reset();
    const found = validateDocument(file, kind);
    setProblem(found);
    if (!found) attach.mutate(file);
  }

  async function download() {
    setDownloadError(null);
    setDownloading(true);
    try {
      saveBlob(await downloadTaxFilingDocument(period, kind), doc?.filename ?? noun);
    } catch (err) {
      setDownloadError(err);
    } finally {
      setDownloading(false);
    }
  }

  return (
    <li className="space-y-2 p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0 flex-1">
          <p className="font-medium">{DOCUMENT_LABEL[kind]}</p>
          {doc ? (
            <p className="mt-0.5 flex items-start gap-2 break-all text-sm text-muted">
              <FileText className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
              <span>
                {doc.filename} · {formatBytes(doc.size)} · subido el {doc.uploaded_at.slice(0, 10)}
              </span>
            </p>
          ) : (
            <p className="mt-0.5 text-sm text-muted">Sin archivo. {DOCUMENT_HINT[kind]}</p>
          )}
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          {doc && (
            <button type="button" onClick={() => void download()} disabled={downloading} aria-label={`Descargar ${noun}`} className={secondaryButton}>
              {downloading ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Download className="h-4 w-4" aria-hidden="true" />}
              Descargar
            </button>
          )}
          <button type="button" onClick={() => inputRef.current?.click()} disabled={attach.isPending} aria-busy={attach.isPending} className={secondaryButton}>
            {attach.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Upload className="h-4 w-4" aria-hidden="true" />}
            {attach.isPending ? "Subiendo…" : doc ? "Reemplazar" : `Subir ${noun}`}
          </button>
          <input ref={inputRef} type="file" accept={DOCUMENT_ACCEPT[kind]} onChange={onPick} aria-label={`Archivo del ${noun}`} tabIndex={-1} className="sr-only" />
        </div>
      </div>
      {problem && <ErrorBanner>{problem}</ErrorBanner>}
      {attach.isError && <ErrorBanner>{describeDocumentError(attach.error)}</ErrorBanner>}
      {downloadError !== null && <ErrorBanner>{describeDocumentError(downloadError)}</ErrorBanner>}
      {attach.isSuccess && (
        <p role="status" className="text-sm">
          {DOCUMENT_LABEL[kind]} guardado.
        </p>
      )}
    </li>
  );
}

/**
 * The acuse and the payment proof of a filing. Files can be attached or
 * replaced on pending and paid filings alike: they change no figure.
 */
export function FilingDocuments({ filing }: { filing: Filing }) {
  return (
    <section aria-labelledby="detail-documents-title" className="space-y-2">
      <h3 id="detail-documents-title" className="text-base font-semibold tracking-tight">
        Documentos
      </h3>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {KINDS.map((kind) => (
          <DocumentRow key={kind} period={filing.period} kind={kind} doc={filing.documents.find((d) => d.kind === kind)} />
        ))}
      </ul>
    </section>
  );
}
