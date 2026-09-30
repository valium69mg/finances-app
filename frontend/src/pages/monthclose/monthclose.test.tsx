import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { MonthClose } from "../../api/monthClose";
import { CloseDetail } from "./CloseDetail";
import { CloseReport } from "./CloseReport";
import { ConfirmDialog } from "./ConfirmDialog";
import { History } from "./History";
import { PreviewPanel } from "./PreviewPanel";
import { describeMonthCloseError } from "./errors";
import { CLOSE } from "./fixtures";
import { closedAtLabel, describeAdjustment, overBudgetOf, signedPercent } from "./labels";

const api = vi.hoisted(() => ({
  previewMonthClose: vi.fn(),
  createMonthClose: vi.fn(),
  listMonthCloses: vi.fn(),
  getMonthClose: vi.fn(),
  deleteMonthClose: vi.fn(),
}));
vi.mock("../../api/monthClose", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/monthClose")>()), ...api }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
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

const apiError = (status: number, code: string, message = "") => new ApiError(status, code, message);

describe("labels", () => {
  it("formats signed percentages", () => {
    expect(signedPercent("50")).toBe("+50.00%");
    expect(signedPercent("-30.5")).toBe("-30.50%");
    expect(signedPercent("abc")).toBe("abc");
  });

  it("describes an adjustment hint in Spanish", () => {
    expect(describeAdjustment(CLOSE.adjustments[0])).toBe("Presupuesto $6,000.00, real $9,000.00 (+50.00%, por encima del presupuesto).");
    expect(describeAdjustment(CLOSE.adjustments[1])).toContain("-50.00%, por debajo");
  });

  it("lists only the over-budget categories", () => {
    expect(overBudgetOf(CLOSE).map((c) => c.category)).toEqual(["Comida"]);
  });

  it("formats the closing instant as a date and leaves garbage untouched", () => {
    expect(closedAtLabel("2026-10-02T15:04:05Z")).toMatch(/^\d{1,2} de \w+ de 2026$/);
    expect(closedAtLabel("nope")).toBe("nope");
  });
});

describe("describeMonthCloseError", () => {
  it("maps the module codes", () => {
    expect(describeMonthCloseError(apiError(400, "invalid_close", "period is in the future"))).toBe("El periodo no es válido: period is in the future");
    expect(describeMonthCloseError(apiError(409, "already_closed"))).toContain("ya está cerrado");
    expect(describeMonthCloseError(apiError(422, "settings_incomplete"))).toContain("Faltan datos en Configuración");
    expect(describeMonthCloseError(apiError(404, "not_found"))).toContain("ya no existe");
  });

  it("falls back to the generic copy", () => {
    expect(describeMonthCloseError(apiError(500, "internal_error"))).toContain("El servidor tuvo un problema");
    expect(describeMonthCloseError(new TypeError("network"))).toContain("No se pudo conectar");
  });
});

describe("CloseReport", () => {
  it("shows totals, the over-budget list, the suggestion, the hints and the filing status", () => {
    wrap(<CloseReport close={CLOSE} />);
    const totals = screen.getByRole("region", { name: "Resumen del mes" });
    expect(totals).toHaveTextContent("$30,000.00");
    expect(totals).toHaveTextContent("$6,860.50");

    const over = screen.getByRole("list", { name: "Categorías sobre presupuesto" });
    expect(within(over).getAllByRole("listitem")).toHaveLength(1);
    expect(over).toHaveTextContent("Comida");
    expect(over).toHaveTextContent("$9,000.00 contra presupuesto de $6,000.00");

    expect(screen.getByText("$20,000.00 de $60,000.00 (33.33%)")).toBeInTheDocument();
    const suggestion = screen.getByRole("list", { name: "Sugerencia del dinero disponible" });
    expect(within(suggestion).getAllByRole("listitem")).toHaveLength(1);
    expect(suggestion).toHaveTextContent("Mover a Fondo de emergencia");
    expect(suggestion).toHaveTextContent("Meta: $60,000.00");

    const hints = screen.getByRole("list", { name: "Ajustes de presupuesto sugeridos" });
    expect(within(hints).getAllByRole("listitem")).toHaveLength(3);
    expect(hints).toHaveTextContent("Inversiones (ahorro)");
    expect(hints).toHaveTextContent("-80.00%");

    expect(screen.getByRole("region", { name: "Declaración del mes" })).toHaveTextContent("Pago pendiente");
  });

  it("splits the suggestion between the fund and Inversiones, or Gastos futuros while paused", () => {
    const split = { ...CLOSE, suggestion: { to_emergency_fund: "1000", to_investments: "2500", to_future_expenses: "0", investments_paused: false } };
    const { unmount } = wrap(<CloseReport close={split} />);
    let list = screen.getByRole("list", { name: "Sugerencia del dinero disponible" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    expect(list).toHaveTextContent("Mover a Inversiones");
    expect(list).not.toHaveTextContent("Gastos futuros");
    unmount();

    const paused = { ...CLOSE, suggestion: { to_emergency_fund: "0", to_investments: "0", to_future_expenses: "3500", investments_paused: true } };
    wrap(<CloseReport close={paused} />);
    list = screen.getByRole("list", { name: "Sugerencia del dinero disponible" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
    expect(list).toHaveTextContent("Mover a Gastos futuros");
    expect(list).toHaveTextContent("Inversiones está en pausa este mes");
    expect(list).not.toHaveTextContent("Mover a Inversiones");
  });

  it("explains the empty cases", () => {
    const empty: MonthClose = { ...CLOSE, categories: [], suggestion: null, adjustments: [], tax_filing_status: null, available: "-5.00" };
    wrap(<CloseReport close={empty} />);
    expect(screen.getByText("Ninguna categoría de gasto excedió su presupuesto.")).toBeInTheDocument();
    expect(screen.getByText(/No hay categorías de gasto en Configuración/)).toBeInTheDocument();
    expect(screen.getByText(/No quedó dinero disponible este mes/)).toBeInTheDocument();
    expect(screen.getByText(/Ningún presupuesto se desvió más de 20%/)).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Declaración del mes" })).toBeNull();
  });

  it("says today's status on a preview and the status at closing time on a stored close", () => {
    const { unmount } = wrap(<CloseReport close={{ ...CLOSE, closed_at: null }} />);
    expect(screen.getByText(/de este periodo hoy/)).toBeInTheDocument();
    unmount();
    wrap(<CloseReport close={CLOSE} />);
    expect(screen.getByText(/al generar el cierre/)).toBeInTheDocument();
  });
});

describe("ConfirmDialog", () => {
  it("confirms, cancels and shows the failure of the last attempt", () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    wrap(
      <ConfirmDialog title="Cerrar septiembre" confirmLabel="Sí" cancelLabel="No" pending={false} error="Falló" onConfirm={onConfirm} onCancel={onCancel}>
        <p>Cuerpo</p>
      </ConfirmDialog>,
    );
    const dialog = screen.getByRole("dialog", { name: "Cerrar septiembre" });
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Falló");
    fireEvent.click(within(dialog).getByRole("button", { name: "Sí" }));
    expect(onConfirm).toHaveBeenCalledOnce();
    fireEvent.click(within(dialog).getByRole("button", { name: "No" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("locks the actions while pending", () => {
    wrap(
      <ConfirmDialog title="T" confirmLabel="Sí" cancelLabel="No" pending onConfirm={() => {}} onCancel={() => {}}>
        <p>x</p>
      </ConfirmDialog>,
    );
    expect(screen.getByRole("button", { name: "Sí" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "No" })).toBeDisabled();
  });
});

describe("PreviewPanel", () => {
  const props = { period: "2026-09", currentMonth: "2026-10", onClosed: vi.fn(), onOpenStored: vi.fn() };

  it("shows the preview and closes the month only after confirming", async () => {
    api.previewMonthClose.mockResolvedValue({ preview: { ...CLOSE, closed_at: null }, existing: null });
    api.createMonthClose.mockResolvedValue(CLOSE);
    const onClosed = vi.fn();
    wrap(<PreviewPanel {...props} onClosed={onClosed} />);

    expect(await screen.findByRole("heading", { name: "Vista previa del cierre de septiembre de 2026" })).toBeInTheDocument();
    expect(api.previewMonthClose).toHaveBeenCalledWith("2026-09");
    fireEvent.click(screen.getByRole("button", { name: "Cerrar mes" }));
    expect(screen.getByRole("dialog", { name: "Cerrar septiembre de 2026" })).toHaveTextContent("foto fija");
    expect(api.createMonthClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Sí, cerrar mes" }));
    await waitFor(() => expect(onClosed).toHaveBeenCalledWith(CLOSE));
    expect(api.createMonthClose).toHaveBeenCalledWith("2026-09");
  });

  it("offers the stored close instead of closing again", async () => {
    api.previewMonthClose.mockResolvedValue({ preview: { ...CLOSE, closed_at: null }, existing: CLOSE });
    const onOpenStored = vi.fn();
    wrap(<PreviewPanel {...props} onOpenStored={onOpenStored} />);

    expect(await screen.findByText(/Este mes ya está cerrado desde el/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cerrar mes" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ver cierre guardado" }));
    expect(onOpenStored).toHaveBeenCalledWith("2026-09");
  });

  it("does not let a future month be closed", async () => {
    api.previewMonthClose.mockResolvedValue({ preview: { ...CLOSE, closed_at: null }, existing: null });
    wrap(<PreviewPanel {...props} period="2026-11" />);
    expect(await screen.findByText(/aún no ha llegado/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cerrar mes" })).toBeDisabled();
  });

  it("keeps the dialog open with the error when storing fails", async () => {
    api.previewMonthClose.mockResolvedValue({ preview: { ...CLOSE, closed_at: null }, existing: null });
    api.createMonthClose.mockRejectedValue(apiError(409, "already_closed"));
    wrap(<PreviewPanel {...props} />);
    fireEvent.click(await screen.findByRole("button", { name: "Cerrar mes" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, cerrar mes" }));
    const dialog = screen.getByRole("dialog");
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("ya está cerrado");
  });

  it("shows a preview error with a retry and a link when settings are incomplete", async () => {
    api.previewMonthClose.mockRejectedValue(apiError(422, "settings_incomplete"));
    wrap(<PreviewPanel {...props} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Faltan datos en Configuración");
    expect(screen.getByRole("link", { name: "Ir a Configuración" })).toHaveAttribute("href", "/configuracion");

    api.previewMonthClose.mockResolvedValue({ preview: { ...CLOSE, closed_at: null }, existing: null });
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByRole("heading", { name: /Vista previa del cierre/ })).toBeInTheDocument();
  });
});

describe("History and CloseDetail", () => {
  const OLDER: MonthClose = { ...CLOSE, period: "2026-08", available: "-150.00", suggestion: null };

  it("lists the closes, selects one and toggles it off", async () => {
    api.listMonthCloses.mockResolvedValue([CLOSE, OLDER]);
    const onSelect = vi.fn();
    wrap(<History selected={null} onSelect={onSelect} onDeleted={() => {}} />);

    const items = within(await screen.findByRole("list", { name: "Cierres guardados" })).getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("septiembre de 2026");
    expect(items[0]).toHaveTextContent("1 categoría excedida");
    expect(items[1]).toHaveTextContent("-$150.00");
    fireEvent.click(screen.getByRole("button", { name: "Ver cierre de septiembre de 2026" }));
    expect(onSelect).toHaveBeenCalledWith("2026-09");
  });

  it("shows the detail of the selected close", async () => {
    api.listMonthCloses.mockResolvedValue([CLOSE]);
    wrap(<History selected="2026-09" onSelect={() => {}} onDeleted={() => {}} />);
    const detail = await screen.findByRole("region", { name: "Detalle del cierre de septiembre de 2026" });
    expect(detail).toHaveTextContent("Estas cifras no cambian");
    expect(screen.getByRole("button", { name: "Ver cierre de septiembre de 2026" })).toHaveAttribute("aria-current", "true");
  });

  it("explains the empty history and the failure with a retry", async () => {
    api.listMonthCloses.mockResolvedValueOnce([]);
    const { unmount } = wrap(<History selected={null} onSelect={() => {}} onDeleted={() => {}} />);
    expect(await screen.findByText(/Aún no has cerrado ningún mes/)).toBeInTheDocument();
    unmount();

    api.listMonthCloses.mockRejectedValueOnce(apiError(500, "internal_error"));
    wrap(<History selected={null} onSelect={() => {}} onDeleted={() => {}} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("El servidor tuvo un problema");
    api.listMonthCloses.mockResolvedValueOnce([]);
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText(/Aún no has cerrado ningún mes/)).toBeInTheDocument();
  });

  it("deletes a close only after confirming, with the snapshot warning", async () => {
    api.deleteMonthClose.mockResolvedValue(undefined);
    const onDeleted = vi.fn();
    wrap(<CloseDetail close={CLOSE} onClose={() => {}} onDeleted={onDeleted} />);

    fireEvent.click(screen.getByRole("button", { name: "Eliminar cierre" }));
    const dialog = screen.getByRole("dialog", { name: "Eliminar el cierre de septiembre de 2026" });
    expect(dialog).toHaveTextContent("Se descarta el resumen guardado");
    expect(dialog).toHaveTextContent("no se tocan");
    fireEvent.click(within(dialog).getByRole("button", { name: "No, conservarlo" }));
    expect(api.deleteMonthClose).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Eliminar cierre" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar cierre" }));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledWith("2026-09"));
    expect(api.deleteMonthClose).toHaveBeenCalledWith("2026-09");
  });

  it("shows a failed delete inside the dialog", async () => {
    api.deleteMonthClose.mockRejectedValue(apiError(404, "not_found"));
    wrap(<CloseDetail close={CLOSE} onClose={() => {}} onDeleted={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Eliminar cierre" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar cierre" }));
    expect(await within(screen.getByRole("dialog")).findByRole("alert")).toHaveTextContent("El cierre ya no existe");
  });
});
