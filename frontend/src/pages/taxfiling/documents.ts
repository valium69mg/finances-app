import { attachTaxFilingDocument, type FilingDocumentKind } from "../../api/taxFiling";
import { formatBytes } from "../invoices/files";
import { describeDocumentError } from "./errors";

/** Same limit as the backend; the server stays the authority, this only saves a round trip. */
export const MAX_DOCUMENT_BYTES = 10 << 20;

export const DOCUMENT_LABEL: Record<FilingDocumentKind, string> = { acuse: "Acuse del SAT", comprobante: "Comprobante de pago" };

/** Lowercase noun used inside sentences ("Subir acuse", "el comprobante no se subió"). */
export const DOCUMENT_NOUN: Record<FilingDocumentKind, string> = { acuse: "acuse", comprobante: "comprobante" };

const EXTENSIONS: Record<FilingDocumentKind, string[]> = {
  acuse: [".pdf"],
  comprobante: [".pdf", ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif"],
};

/** `accept` attribute of the file picker of each kind. */
export const DOCUMENT_ACCEPT: Record<FilingDocumentKind, string> = {
  acuse: ".pdf,application/pdf",
  comprobante: ".pdf,.jpg,.jpeg,.png,.webp,.heic,.heif,application/pdf,image/jpeg,image/png,image/webp,image/heic,image/heif",
};

export const DOCUMENT_HINT: Record<FilingDocumentKind, string> = {
  acuse: "Solo PDF, hasta 10 MB.",
  comprobante: "PDF o imagen (JPG, PNG, WebP o HEIC), hasta 10 MB.",
};

/** Returns a Spanish message when the file cannot be uploaded as `kind`, or null when it looks fine. */
export function validateDocument(file: File, kind: FilingDocumentKind): string | null {
  const name = file.name.toLowerCase();
  if (!EXTENSIONS[kind].some((ext) => name.endsWith(ext))) {
    return kind === "acuse" ? "El acuse debe ser un PDF (extensión .pdf)." : "El comprobante debe ser un PDF o una imagen JPG, PNG, WebP o HEIC.";
  }
  if (file.size === 0) return "El archivo está vacío.";
  if (file.size > MAX_DOCUMENT_BYTES) return `El archivo pesa ${formatBytes(file.size)}; el máximo es ${formatBytes(MAX_DOCUMENT_BYTES)}.`;
  return null;
}

export interface UploadFailure {
  kind: FilingDocumentKind;
  message: string;
}

/**
 * Uploads the chosen files one after the other, after the filing or the payment
 * they belong to was already saved. A failure never throws: it is returned so
 * the caller can say the record was saved but the file was not.
 */
export async function uploadDocuments(period: string, files: { kind: FilingDocumentKind; file: File }[]): Promise<UploadFailure[]> {
  const failures: UploadFailure[] = [];
  for (const { kind, file } of files) {
    try {
      await attachTaxFilingDocument(period, kind, file);
    } catch (err) {
      failures.push({ kind, message: describeDocumentError(err) });
    }
  }
  return failures;
}

/** Spanish sentence for failed uploads: what was saved, what was not and where to retry. */
export function uploadFailureMessage(saved: string, failures: UploadFailure[]): string {
  const nouns = failures.map((f) => `el ${DOCUMENT_NOUN[f.kind]}`).join(" y ");
  const reasons = failures.map((f) => f.message).join(" ");
  const retry = failures.length > 1 ? "Súbelos" : "Súbelo";
  return `${saved}, pero no se pudo subir ${nouns}. ${reasons} ${retry} desde el detalle en Declaraciones presentadas.`;
}

/** Saves a downloaded blob through a temporary link; the object URL is released right after. */
export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
