import type { DocumentKind } from "../../api/invoices";

/** Same limits as the backend; the server stays the authority, this only saves a round trip. */
export const MAX_BYTES: Record<DocumentKind, number> = { xml: 1 << 20, pdf: 10 << 20 };

export const KIND_LABEL: Record<DocumentKind, string> = { xml: "XML", pdf: "PDF" };

/** Returns a Spanish message when the file cannot be uploaded as `kind`, or null when it looks fine. */
export function validateFile(file: File, kind: DocumentKind): string | null {
  if (!file.name.toLowerCase().endsWith(`.${kind}`)) return `El archivo debe ser un ${KIND_LABEL[kind]} (extensión .${kind}).`;
  if (file.size === 0) return "El archivo está vacío.";
  if (file.size > MAX_BYTES[kind]) return `El ${KIND_LABEL[kind]} pesa ${formatBytes(file.size)}; el máximo es ${formatBytes(MAX_BYTES[kind])}.`;
  return null;
}

/** "1.5 MB", "820 KB", "12 B" (binary units, one decimal for MB). */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** CFDI UUID in the 8-4-4-4-12 hexadecimal format (case-insensitive, surrounding spaces ignored). */
export const isValidUuid = (v: string) => UUID_PATTERN.test(v.trim());
