import { useEffect, useState } from "react";
import { Download, Loader2 } from "lucide-react";
import { Modal } from "./Modal";
import { parseCfdi, prettyXml } from "../lib/cfdi";
import { saveBlob } from "../lib/saveBlob";
import { formatMoney } from "../pages/expenses/money";
import { ErrorBanner, secondaryButton } from "../pages/settings/ui";

type Kind = "pdf" | "image" | "xml" | "other";

const IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp", "image/heic", "image/heif"];
const IMAGE_EXT = /\.(jpe?g|png|webp|heic|heif)$/i;

/** What the preview can show: the stored content type wins, the file extension covers a generic one. */
export function previewKind(contentType: string, name: string): Kind {
  const type = contentType.split(";")[0].trim().toLowerCase();
  if (type === "application/pdf") return "pdf";
  if (IMAGE_TYPES.includes(type)) return "image";
  if (type === "application/xml" || type === "text/xml") return "xml";
  if (/\.pdf$/i.test(name)) return "pdf";
  if (IMAGE_EXT.test(name)) return "image";
  if (/\.xml$/i.test(name)) return "xml";
  return "other";
}

interface Props {
  /** Fetches the file (the authenticated download of the page). */
  load: () => Promise<Blob>;
  name: string;
  contentType: string;
  onClose: () => void;
  /** Spanish message for a failed load. */
  describeError: (err: unknown) => string;
}

type State =
  | { status: "loading" }
  | { status: "error"; error: unknown }
  | { status: "ready"; blob: Blob; url: string | null; text: string | null };

const Unavailable = () => <p className="text-sm text-muted">Vista previa no disponible en este navegador.</p>;

function CfdiView({ text }: { text: string }) {
  const summary = parseCfdi(text);
  if (!summary) return <RawXml text={text} open />;
  const money = (v?: string) => (v ? formatMoney(v) : undefined);
  const rows: [string, string | undefined][] = [
    ["Emisor", [summary.emisorNombre, summary.emisorRfc].filter(Boolean).join(" · ") || undefined],
    ["Receptor", [summary.receptorNombre, summary.receptorRfc].filter(Boolean).join(" · ") || undefined],
    ["Fecha", summary.fecha],
    ["Serie y folio", [summary.serie, summary.folio].filter(Boolean).join(" ") || undefined],
    ["Subtotal", money(summary.subTotal)],
    ["Total", money(summary.total)],
    ["Moneda", summary.moneda],
    ["UUID", summary.uuid],
  ];
  return (
    <div className="space-y-4">
      <dl className="grid gap-x-4 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
        {rows
          .filter(([, v]) => v)
          .map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted">{k}</dt>
              <dd className="break-all font-medium">{v}</dd>
            </div>
          ))}
      </dl>
      <RawXml text={text} />
    </div>
  );
}

function RawXml({ text, open = false }: { text: string; open?: boolean }) {
  return (
    <details open={open} className="rounded-lg border border-border">
      <summary className="focus-ring flex min-h-11 cursor-pointer items-center px-3 text-sm font-medium">Ver XML</summary>
      <pre className="max-h-[40dvh] overflow-auto whitespace-pre-wrap break-all border-t border-border p-3 text-xs">{prettyXml(text)}</pre>
    </details>
  );
}

/**
 * In-page preview of an attached document in a modal: PDF in an iframe, images in an img, a CFDI XML as a summary
 * plus its raw text, anything else as a "no preview" note. The file is fetched with the page's authenticated loader
 * and shown through an object URL that is revoked when the modal closes. It never opens a tab or window.
 */
export function DocumentPreview({ load, name, contentType, onClose, describeError }: Props) {
  const kind = previewKind(contentType, name);
  const [state, setState] = useState<State>({ status: "loading" });
  const [imageFailed, setImageFailed] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);

  useEffect(() => {
    let cancelled = false;
    let url: string | null = null;
    setState({ status: "loading" });
    (async () => {
      try {
        const blob = await load();
        if (cancelled) return;
        let text: string | null = null;
        if (kind === "xml") text = await blob.text();
        else if (kind === "pdf") url = URL.createObjectURL(blob.type === "application/pdf" ? blob : new Blob([blob], { type: "application/pdf" }));
        else if (kind === "image") url = URL.createObjectURL(blob);
        if (cancelled) {
          if (url) URL.revokeObjectURL(url);
          return;
        }
        setState({ status: "ready", blob, url, text });
      } catch (error) {
        if (!cancelled) setState({ status: "error", error });
      }
    })();
    return () => {
      cancelled = true;
      if (url) URL.revokeObjectURL(url);
    };
    // The loader is a fresh closure on every render of the page: the preview loads once per mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function download() {
    setSaveError(null);
    setSaving(true);
    try {
      saveBlob(state.status === "ready" ? state.blob : await load(), name);
    } catch (err) {
      setSaveError(err);
    } finally {
      setSaving(false);
    }
  }

  let body;
  if (state.status === "loading") {
    body = (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Cargando vista previa…
      </p>
    );
  } else if (state.status === "error") {
    body = <ErrorBanner>{describeError(state.error)}</ErrorBanner>;
  } else if (kind === "pdf" && state.url) {
    body = <iframe src={state.url} title={`Vista previa de ${name}`} className="h-[70vh] h-[70dvh] w-full rounded-lg border border-border bg-surface" />;
  } else if (kind === "image" && state.url && !imageFailed) {
    body = <img src={state.url} alt={name} onError={() => setImageFailed(true)} className="mx-auto max-h-[70vh] max-h-[70dvh] max-w-full object-contain" />;
  } else if (kind === "xml" && state.text !== null) {
    body = <CfdiView text={state.text} />;
  } else {
    body = <Unavailable />;
  }

  return (
    <Modal title={name} onClose={onClose} wide>
      <div className="space-y-4">
        {body}
        {saveError !== null && <ErrorBanner>{describeError(saveError)}</ErrorBanner>}
        <button type="button" onClick={() => void download()} disabled={saving} className={secondaryButton}>
          {saving ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Download className="h-4 w-4" aria-hidden="true" />}
          Descargar
        </button>
      </div>
    </Modal>
  );
}

