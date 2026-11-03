/** Readable summary of a CFDI (3.3 or 4.0). Every field is optional: a missing node never throws. */
export interface CfdiSummary {
  emisorNombre?: string;
  emisorRfc?: string;
  receptorNombre?: string;
  receptorRfc?: string;
  fecha?: string;
  serie?: string;
  folio?: string;
  subTotal?: string;
  total?: string;
  moneda?: string;
  uuid?: string;
}

function firstByLocalName(doc: Document, name: string): Element | undefined {
  return doc.getElementsByTagNameNS("*", name)[0];
}

function attr(el: Element | undefined, name: string): string | undefined {
  const v = el?.getAttribute(name)?.trim();
  return v ? v : undefined;
}

/**
 * Extracts the summary of a CFDI XML by matching local names, so the 3.3 and 4.0 namespaces both work.
 * Returns null when the text is not well-formed XML or has no cfdi:Comprobante root.
 */
export function parseCfdi(xml: string): CfdiSummary | null {
  let doc: Document;
  try {
    doc = new DOMParser().parseFromString(xml, "application/xml");
  } catch {
    return null;
  }
  if (doc.getElementsByTagName("parsererror").length > 0) return null;
  const root = doc.documentElement;
  if (!root || root.localName !== "Comprobante") return null;
  const emisor = firstByLocalName(doc, "Emisor");
  const receptor = firstByLocalName(doc, "Receptor");
  const timbre = firstByLocalName(doc, "TimbreFiscalDigital");
  // Attribute names differ in case between versions (Serie/serie, Folio/folio in some old samples): try both.
  const pick = (el: Element | undefined, name: string) => attr(el, name) ?? attr(el, name.toLowerCase());
  return {
    emisorNombre: pick(emisor, "Nombre"),
    emisorRfc: pick(emisor, "Rfc"),
    receptorNombre: pick(receptor, "Nombre"),
    receptorRfc: pick(receptor, "Rfc"),
    fecha: pick(root, "Fecha"),
    serie: pick(root, "Serie"),
    folio: pick(root, "Folio"),
    subTotal: pick(root, "SubTotal"),
    total: pick(root, "Total"),
    moneda: pick(root, "Moneda"),
    uuid: pick(timbre, "UUID")?.toUpperCase(),
  };
}

/** Re-indents well-formed XML for display; returns the input untouched when it cannot be parsed. */
export function prettyXml(xml: string): string {
  const doc = new DOMParser().parseFromString(xml, "application/xml");
  if (doc.getElementsByTagName("parsererror").length > 0) return xml;
  const lines: string[] = [];
  const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/"/g, "&quot;");
  const open = (el: Element) => `<${el.tagName}${Array.from(el.attributes).map((a) => ` ${a.name}="${esc(a.value)}"`).join("")}`;
  function walk(el: Element, depth: number) {
    const pad = "  ".repeat(depth);
    const kids = Array.from(el.children);
    const text = kids.length === 0 ? (el.textContent ?? "").trim() : "";
    if (kids.length === 0 && !text) lines.push(`${pad}${open(el)}/>`);
    else if (kids.length === 0) lines.push(`${pad}${open(el)}>${esc(text)}</${el.tagName}>`);
    else {
      lines.push(`${pad}${open(el)}>`);
      kids.forEach((k) => walk(k, depth + 1));
      lines.push(`${pad}</${el.tagName}>`);
    }
  }
  walk(doc.documentElement, 0);
  return lines.join("\n");
}
