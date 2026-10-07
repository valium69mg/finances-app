import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { FilingDocument } from "../../api/taxFiling";
import { DOCUMENT_ACCEPT, uploadDocuments, uploadFailureMessage, validateDocument } from "./documents";
import { describeDocumentError, describeTaxFilingError } from "./errors";
import { FILING, PAID_FILING, PREVIEW } from "./fixtures";
import { FilingDocuments } from "./FilingDocuments";
import { FilingHistory } from "./FilingHistory";
import { PaymentDialog } from "./PaymentDialog";
import { RegisterForm } from "./RegisterForm";

const api = vi.hoisted(() => ({
  registerTaxFiling: vi.fn(),
  payTaxFiling: vi.fn(),
  listTaxFilings: vi.fn(),
  attachTaxFilingDocument: vi.fn(),
  downloadTaxFilingDocument: vi.fn(),
}));
vi.mock("../../api/taxFiling", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/taxFiling")>()), ...api }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const pdf = (name = "acuse.pdf", body = "%PDF-1.4") => new File([body], name, { type: "application/pdf" });
const photo = (name = "IMG_0001.HEIC") => new File(["img"], name, { type: "image/heic" });
const pick = (label: string | RegExp, file: File) => fireEvent.change(screen.getByLabelText(label), { target: { files: [file] } });

const ACUSE_DOC: FilingDocument = { kind: "acuse", filename: "acuse-oct.pdf", content_type: "application/pdf", size: 2048, uploaded_at: "2026-11-06T09:00:00Z" };

describe("validateDocument", () => {
  it("accepts a PDF acuse and rejects anything else", () => {
    expect(validateDocument(pdf(), "acuse")).toBeNull();
    expect(validateDocument(pdf("ACUSE.PDF"), "acuse")).toBeNull();
    expect(validateDocument(photo("a.jpg"), "acuse")).toContain("PDF");
  });

  it("accepts a PDF or a phone photo as comprobante", () => {
    for (const name of ["p.pdf", "p.jpg", "p.JPEG", "p.png", "p.webp", "IMG_1.HEIC", "p.heif"]) expect(validateDocument(photo(name), "comprobante"), name).toBeNull();
    expect(validateDocument(photo("p.gif"), "comprobante")).toContain("PDF o una imagen");
  });

  it("rejects empty and oversized files", () => {
    expect(validateDocument(new File([], "a.pdf"), "acuse")).toBe("El archivo está vacío.");
    const big = new File([new Uint8Array((10 << 20) + 1)], "a.pdf");
    expect(validateDocument(big, "acuse")).toContain("el máximo es 10.0 MB");
  });

  it("limits the pickers with the accept attribute", () => {
    expect(DOCUMENT_ACCEPT.acuse).toBe(".pdf,application/pdf");
    expect(DOCUMENT_ACCEPT.comprobante).toContain("image/heic");
  });
});

describe("uploadDocuments", () => {
  it("uploads sequentially and reports the failures instead of throwing", async () => {
    const order: string[] = [];
    api.attachTaxFilingDocument.mockImplementation(async (_p: string, kind: string) => {
      order.push(kind);
      if (kind === "acuse") throw new ApiError(503, "storage_unavailable");
      return FILING;
    });
    const failures = await uploadDocuments("2026-10", [
      { kind: "acuse", file: pdf() },
      { kind: "comprobante", file: pdf("pago.pdf") },
    ]);
    expect(order).toEqual(["acuse", "comprobante"]);
    expect(failures).toHaveLength(1);
    expect(failures[0].kind).toBe("acuse");
    expect(failures[0].message).toContain("almacenamiento de documentos no está disponible");
  });

  it("says the record was saved but the file was not", () => {
    const msg = uploadFailureMessage("La declaración se guardó", [{ kind: "acuse", message: "Falló." }]);
    expect(msg).toBe("La declaración se guardó, pero no se pudo subir el acuse. Falló. Súbelo desde el detalle en Declaraciones presentadas.");
    expect(uploadFailureMessage("Guardado", [{ kind: "acuse", message: "A." }, { kind: "comprobante", message: "B." }])).toContain("el acuse y el comprobante");
  });
});

describe("document errors", () => {
  it("maps the upload codes to Spanish", () => {
    expect(describeTaxFilingError(new ApiError(400, "invalid_document", "the acuse must be a PDF"))).toBe("El archivo no es válido: the acuse must be a PDF");
    expect(describeTaxFilingError(new ApiError(413, "request_too_large"))).toContain("demasiado grande");
    expect(describeTaxFilingError(new ApiError(503, "storage_unavailable"))).toContain("almacenamiento");
    expect(describeDocumentError(new ApiError(404, "not_found"))).toContain("El archivo o la declaración ya no existe");
    expect(describeDocumentError(new ApiError(413, "x"))).toContain("demasiado grande");
  });
});

describe("RegisterForm documents", () => {
  const registered = { filing: { ...FILING }, warnings: [] };

  it("shows the acuse picker always and the comprobante picker only when paid", () => {
    wrap(<RegisterForm preview={PREVIEW} onRegistered={vi.fn()} />);
    expect(screen.getByLabelText("Acuse (PDF)")).toHaveAttribute("accept", DOCUMENT_ACCEPT.acuse);
    expect(screen.queryByLabelText("Comprobante de pago (imagen o PDF)")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }));
    expect(screen.getByLabelText("Comprobante de pago (imagen o PDF)")).toHaveAttribute("accept", DOCUMENT_ACCEPT.comprobante);
  });

  it("uploads the chosen files after the filing is saved", async () => {
    api.registerTaxFiling.mockResolvedValue(registered);
    api.attachTaxFilingDocument.mockResolvedValue(FILING);
    const onRegistered = vi.fn();
    wrap(<RegisterForm preview={PREVIEW} onRegistered={onRegistered} />);
    const acuse = pdf();
    const proof = photo();
    pick("Acuse (PDF)", acuse);
    fireEvent.click(screen.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }));
    pick("Comprobante de pago (imagen o PDF)", proof);
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));

    await waitFor(() => expect(onRegistered).toHaveBeenCalledWith(registered, []));
    expect(api.attachTaxFilingDocument.mock.calls).toEqual([
      ["2026-10", "acuse", acuse],
      ["2026-10", "comprobante", proof],
    ]);
    // The filing is JSON: files never travel with it.
    expect(api.registerTaxFiling.mock.calls[0][0]).not.toHaveProperty("file");
  });

  it("keeps the filing and reports the failed upload", async () => {
    api.registerTaxFiling.mockResolvedValue(registered);
    api.attachTaxFilingDocument.mockRejectedValue(new ApiError(503, "storage_unavailable"));
    const onRegistered = vi.fn();
    wrap(<RegisterForm preview={PREVIEW} onRegistered={onRegistered} />);
    pick("Acuse (PDF)", pdf());
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));

    await waitFor(() => expect(onRegistered).toHaveBeenCalled());
    const [result, failures] = onRegistered.mock.calls[0];
    expect(result).toBe(registered);
    expect(failures).toEqual([{ kind: "acuse", message: expect.stringContaining("almacenamiento") }]);
  });

  it("drops the comprobante when the payment is unchecked", async () => {
    api.registerTaxFiling.mockResolvedValue(registered);
    wrap(<RegisterForm preview={PREVIEW} onRegistered={vi.fn()} />);
    fireEvent.click(screen.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }));
    pick("Comprobante de pago (imagen o PDF)", photo());
    fireEvent.click(screen.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }));
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));
    await waitFor(() => expect(api.registerTaxFiling).toHaveBeenCalled());
    expect(api.attachTaxFilingDocument).not.toHaveBeenCalled();
  });

  it("validates the files before registering anything", () => {
    wrap(<RegisterForm preview={PREVIEW} onRegistered={vi.fn()} />);
    pick("Acuse (PDF)", photo("foto.jpg"));
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));
    expect(screen.getByText("El acuse debe ser un PDF (extensión .pdf).")).toBeInTheDocument();
    expect(api.registerTaxFiling).not.toHaveBeenCalled();
  });
});

describe("PaymentDialog comprobante", () => {
  it("uploads the proof after the payment is saved", async () => {
    const result = { filing: { ...FILING, status: "pagada" as const }, warnings: [] };
    api.payTaxFiling.mockResolvedValue(result);
    api.attachTaxFilingDocument.mockResolvedValue(result.filing);
    const onPaid = vi.fn();
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={onPaid} />);
    const proof = photo();
    pick("Comprobante de pago (opcional)", proof);
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-11-12" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));

    await waitFor(() => expect(onPaid).toHaveBeenCalledWith(result, []));
    expect(api.attachTaxFilingDocument).toHaveBeenCalledWith("2026-10", "comprobante", proof);
  });

  it("does not upload anything without a file and reports a failed upload", async () => {
    const result = { filing: { ...FILING, status: "pagada" as const }, warnings: [] };
    api.payTaxFiling.mockResolvedValue(result);
    const onPaid = vi.fn();
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={onPaid} />);
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-11-12" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(onPaid).toHaveBeenCalledWith(result, []));
    expect(api.attachTaxFilingDocument).not.toHaveBeenCalled();

    cleanup();
    api.attachTaxFilingDocument.mockRejectedValue(new ApiError(400, "invalid_document", "bad"));
    const onPaid2 = vi.fn();
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={onPaid2} />);
    pick("Comprobante de pago (opcional)", photo());
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-11-12" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(onPaid2).toHaveBeenCalled());
    expect(onPaid2.mock.calls[0][1]).toEqual([{ kind: "comprobante", message: "El archivo no es válido: bad" }]);
  });

  it("rejects an unsupported proof before paying", () => {
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={vi.fn()} />);
    pick("Comprobante de pago (opcional)", new File(["x"], "pago.txt"));
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(screen.getByText(/PDF o una imagen/)).toBeInTheDocument();
    expect(api.payTaxFiling).not.toHaveBeenCalled();
  });
});

describe("FilingDocuments", () => {
  it("offers to upload each missing document", () => {
    wrap(<FilingDocuments filing={FILING} />);
    expect(screen.getByRole("button", { name: "Subir acuse" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Subir comprobante" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Descargar/ })).not.toBeInTheDocument();
  });

  it("uploads a missing acuse and refreshes", async () => {
    api.attachTaxFilingDocument.mockResolvedValue(FILING);
    wrap(<FilingDocuments filing={FILING} />);
    const file = pdf();
    pick("Archivo del acuse", file);
    await waitFor(() => expect(api.attachTaxFilingDocument).toHaveBeenCalledWith("2026-10", "acuse", file));
    expect(await screen.findByRole("status")).toHaveTextContent("Acuse del SAT guardado.");
  });

  it("shows the file with download and replace on a paid filing", async () => {
    const paid = { ...PAID_FILING, documents: [ACUSE_DOC] };
    api.attachTaxFilingDocument.mockResolvedValue(paid);
    wrap(<FilingDocuments filing={paid} />);
    expect(screen.getByText(/acuse-oct\.pdf · 2 KB · subido el 2026-11-06/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reemplazar" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Subir comprobante" })).toBeInTheDocument();
    const replacement = pdf("nuevo.pdf");
    pick("Archivo del acuse", replacement);
    await waitFor(() => expect(api.attachTaxFilingDocument).toHaveBeenCalledWith("2026-09", "acuse", replacement));
  });

  it("downloads through the API as a blob and saves it with its file name", async () => {
    api.downloadTaxFilingDocument.mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    const create = vi.fn(() => "blob:fake");
    const revoke = vi.fn();
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke }));
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    wrap(<FilingDocuments filing={{ ...FILING, documents: [ACUSE_DOC] }} />);
    fireEvent.click(screen.getByRole("button", { name: "Descargar acuse" }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(api.downloadTaxFilingDocument).toHaveBeenCalledWith("2026-10", "acuse");
    expect(create).toHaveBeenCalled();
    click.mockRestore();
    vi.unstubAllGlobals();
  });

  it("validates the file and shows the API error", async () => {
    wrap(<FilingDocuments filing={FILING} />);
    pick("Archivo del acuse", photo("foto.jpg"));
    expect(screen.getByRole("alert")).toHaveTextContent("El acuse debe ser un PDF");
    expect(api.attachTaxFilingDocument).not.toHaveBeenCalled();

    api.attachTaxFilingDocument.mockRejectedValue(new ApiError(503, "storage_unavailable"));
    pick("Archivo del comprobante", pdf("pago.pdf"));
    expect(await screen.findByText(/almacenamiento de documentos no está disponible/)).toBeInTheDocument();
  });

  it("shows a download error", async () => {
    api.downloadTaxFilingDocument.mockRejectedValue(new ApiError(404, "not_found"));
    wrap(<FilingDocuments filing={{ ...FILING, documents: [ACUSE_DOC] }} />);
    fireEvent.click(screen.getByRole("button", { name: "Descargar acuse" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("El archivo o la declaración ya no existe");
  });
});

describe("FilingHistory documents column", () => {
  it("lists which documents each filing has", async () => {
    api.listTaxFilings.mockResolvedValue([{ ...FILING, documents: [ACUSE_DOC] }, PAID_FILING]);
    wrap(<FilingHistory filter={{}} selectedPeriod={null} onSelect={vi.fn()} onPay={vi.fn()} />);
    const rows = await screen.findAllByRole("row");
    expect(within(rows[1]).getByText("Acuse del SAT")).toBeInTheDocument();
    expect(within(rows[2]).queryByText("Acuse del SAT")).not.toBeInTheDocument();
  });
});
