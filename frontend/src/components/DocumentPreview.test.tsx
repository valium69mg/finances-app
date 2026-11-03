import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DocumentPreview, previewKind } from "./DocumentPreview";

const saveBlob = vi.hoisted(() => vi.fn());
vi.mock("../lib/saveBlob", () => ({ saveBlob }));

const XML = `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Serie="A" Folio="7" Fecha="2026-10-02T10:00:00" SubTotal="1000" Moneda="USD" Total="1234.5">
  <cfdi:Emisor Rfc="EMI010101AAA" Nombre="Emisor SA"/><cfdi:Receptor Rfc="REC020202BBB" Nombre="Cliente LLC"/>
  <cfdi:Complemento><tfd:TimbreFiscalDigital UUID="6F1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D"/></cfdi:Complemento>
</cfdi:Comprobante>`;

let created: string[];
let revoked: string[];

beforeEach(() => {
  created = [];
  revoked = [];
  let n = 0;
  URL.createObjectURL = vi.fn(() => {
    const u = `blob:test/${++n}`;
    created.push(u);
    return u;
  });
  URL.revokeObjectURL = vi.fn((u: string) => void revoked.push(u));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function setup(blob: Blob, name: string, contentType: string, load = vi.fn().mockResolvedValue(blob)) {
  const onClose = vi.fn();
  const view = render(<DocumentPreview load={load} name={name} contentType={contentType} onClose={onClose} describeError={() => "No se pudo descargar."} />);
  return { onClose, load, ...view };
}

describe("previewKind", () => {
  it("uses the content type first and the extension as a fallback", () => {
    expect(previewKind("application/pdf", "a")).toBe("pdf");
    expect(previewKind("image/heic", "a")).toBe("image");
    expect(previewKind("application/xml; charset=utf-8", "a")).toBe("xml");
    expect(previewKind("application/octet-stream", "IMG.PNG")).toBe("image");
    expect(previewKind("application/octet-stream", "a.bin")).toBe("other");
  });
});

describe("DocumentPreview", () => {
  it("shows a PDF in an iframe with an object URL and a title", async () => {
    setup(new Blob(["%PDF"], { type: "application/octet-stream" }), "factura.pdf", "application/pdf");
    const frame = await screen.findByTitle("Vista previa de factura.pdf");
    expect(frame.tagName).toBe("IFRAME");
    expect(frame).toHaveAttribute("src", created[0]);
  });

  it("shows an image and falls back with a download button when it fails to load", async () => {
    setup(new Blob(["x"], { type: "image/heic" }), "IMG_1.HEIC", "image/heic");
    const img = await screen.findByAltText("IMG_1.HEIC");
    expect(img).toHaveAttribute("src", created[0]);
    fireEvent.error(img);
    expect(await screen.findByText("Vista previa no disponible en este navegador.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Descargar" })).toBeInTheDocument();
  });

  it("summarises a CFDI and keeps the raw XML as plain text", async () => {
    const { container } = setup(new Blob([XML], { type: "application/xml" }), "cfdi.xml", "application/xml");
    expect(await screen.findByText("Emisor SA · EMI010101AAA")).toBeInTheDocument();
    expect(screen.getByText("Cliente LLC · REC020202BBB")).toBeInTheDocument();
    expect(screen.getByText("A 7")).toBeInTheDocument();
    expect(screen.getByText("$1,000.00")).toBeInTheDocument();
    expect(screen.getByText("$1,234.50")).toBeInTheDocument();
    expect(screen.getByText("USD")).toBeInTheDocument();
    expect(screen.getByText("6F1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D")).toBeInTheDocument();
    expect(screen.getByText("Ver XML")).toBeInTheDocument();
    expect(container.ownerDocument.querySelector("pre")?.textContent).toContain("<cfdi:Emisor");
    expect(created).toHaveLength(0);
  });

  it("shows only the raw text when the XML does not parse", async () => {
    setup(new Blob(["<broken"], { type: "application/xml" }), "x.xml", "application/xml");
    expect(await screen.findByText("<broken")).toBeInTheDocument();
  });

  it("falls back for an unknown type", async () => {
    setup(new Blob(["x"]), "a.bin", "application/octet-stream");
    expect(await screen.findByText("Vista previa no disponible en este navegador.")).toBeInTheDocument();
  });

  it("saves the loaded blob through the download button", async () => {
    const blob = new Blob(["%PDF"], { type: "application/pdf" });
    const { load } = setup(blob, "factura.pdf", "application/pdf");
    await screen.findByTitle("Vista previa de factura.pdf");
    fireEvent.click(screen.getByRole("button", { name: "Descargar" }));
    await waitFor(() => expect(saveBlob).toHaveBeenCalledTimes(1));
    expect(saveBlob.mock.calls[0][0]).toBe(blob);
    expect(saveBlob.mock.calls[0][1]).toBe("factura.pdf");
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("shows the described error when the load fails", async () => {
    setup(new Blob([]), "a.pdf", "application/pdf", vi.fn().mockRejectedValue(new Error("boom")));
    expect(await screen.findByText("No se pudo descargar.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Descargar" })).toBeInTheDocument();
  });

  it("revokes the object URL when it closes", async () => {
    const { unmount } = setup(new Blob(["%PDF"], { type: "application/pdf" }), "factura.pdf", "application/pdf");
    await screen.findByTitle("Vista previa de factura.pdf");
    expect(revoked).toEqual([]);
    unmount();
    expect(revoked).toEqual(created);
    expect(created).toHaveLength(1);
  });
});
