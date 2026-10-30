import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { WishItem } from "../../api/wishlist";
import { ListaDeDeseos } from "../ListaDeDeseos";

const api = vi.hoisted(() => ({
  listWishlist: vi.fn(),
  createWishItem: vi.fn(),
  updateWishItem: vi.fn(),
  deleteWishItem: vi.fn(),
  convertWishToFuture: vi.fn(),
  convertWishToExpense: vi.fn(),
  convertWishToRequest: vi.fn(),
  listPersonalCategories: vi.fn(),
  getRequestCategories: vi.fn(),
}));
vi.mock("../../api/wishlist", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/wishlist")>()), ...api }));
vi.mock("../../api/personal", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/personal")>()), listPersonalCategories: api.listPersonalCategories }));
vi.mock("../../api/expenseRequests", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/expenseRequests")>()), getRequestCategories: api.getRequestCategories }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const base: WishItem = {
  id: 1,
  name: "Bolsa",
  price: "499.90",
  category_id: 5,
  category: "Ropa",
  store_url: "https://tienda.example/b",
  priority: "alta",
  note: "roja",
  status: "activa",
  conversion: null,
  created_at: "2026-10-01T12:00:00Z",
  updated_at: "2026-10-01T12:00:00Z",
};
const item = (patch: Partial<WishItem>): WishItem => ({ ...base, ...patch });

const CATEGORIES = [
  { id: 5, kind: "Gasto", name: "Ropa", fixed: false },
  { id: 6, kind: "Gasto", name: "Salud", fixed: false },
];

function renderPage(items: WishItem[]) {
  api.listWishlist.mockResolvedValue(items);
  api.listPersonalCategories.mockResolvedValue(CATEGORIES);
  api.getRequestCategories.mockResolvedValue(["Personal", "Comida"]);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ListaDeDeseos />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ListaDeDeseos list", () => {
  it("separates active and converted items and shows what each became", async () => {
    renderPage([
      item({ id: 1, name: "Bolsa" }),
      item({ id: 2, name: "Zapatos", status: "convertida", priority: "baja", conversion: { type: "gasto_futuro", target_id: 9, request_state: null } }),
      item({ id: 3, name: "Labial", status: "convertida", conversion: { type: "peticion", target_id: 4, request_state: "solicitada" } }),
    ]);
    const active = await screen.findByRole("list", { name: "Deseos activos" });
    expect(within(active).getByText("Bolsa")).toBeInTheDocument();
    expect(within(active).queryByText("Zapatos")).not.toBeInTheDocument();
    const converted = screen.getByRole("list", { name: "Deseos convertidos" });
    // The card says what each became; the links live in the detail dialog.
    expect(within(converted).getByText("Se convirtió en un gasto futuro.")).toBeInTheDocument();
    expect(within(converted).getByTestId("request-state")).toHaveTextContent("Solicitada");
    expect(within(converted).queryByRole("link")).not.toBeInTheDocument();
    expect(within(converted).queryByRole("button", { name: /Convertir/ })).not.toBeInTheDocument();

    fireEvent.click(within(converted).getByRole("button", { name: "Ver Zapatos" }));
    let dialog = screen.getByRole("dialog", { name: "Zapatos" });
    expect(within(dialog).getByRole("link", { name: "Ver en Mis gastos futuros" })).toHaveAttribute("href", "/mis-gastos-futuros");
    expect(within(dialog).queryByRole("button", { name: /Convertir/ })).not.toBeInTheDocument();
    fireEvent.keyDown(dialog, { key: "Escape" });
    fireEvent.click(within(converted).getByRole("button", { name: "Ver Labial" }));
    dialog = screen.getByRole("dialog", { name: "Labial" });
    expect(within(dialog).getByRole("link", { name: "Ver en Peticiones" })).toHaveAttribute("href", "/peticiones");
  });

  it("shows priority with text, the total of the active items and a safe store link", async () => {
    renderPage([item({ id: 1, price: "100.10" }), item({ id: 2, name: "Falda", price: "50", priority: "baja", store_url: null, note: null })]);
    await screen.findByText("Bolsa");
    expect(screen.getAllByTestId("wish-priority").map((n) => n.textContent)).toEqual(["Prioridad alta", "Prioridad baja"]);
    expect(screen.getByText("$150.10 MXN")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Ver en la tienda/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Ver Bolsa" }));
    const link = screen.getByRole("link", { name: /Ver en la tienda/ });
    expect(link).toHaveAttribute("href", "https://tienda.example/b");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getAllByRole("link", { name: /Ver en la tienda/ })).toHaveLength(1);
  });

  it("never renders a link that is not http or https", async () => {
    renderPage([item({ store_url: "javascript:alert(1)" })]);
    fireEvent.click(await screen.findByRole("button", { name: "Ver Bolsa" }));
    expect(screen.queryByRole("link", { name: /Ver en la tienda/ })).not.toBeInTheDocument();
  });

  it("says why an active item came back after its request was rejected or cancelled", async () => {
    renderPage([
      item({ id: 1, name: "Bolsa", conversion: { type: "peticion", target_id: 4, request_state: "rechazada" } }),
      item({ id: 2, name: "Falda", conversion: { type: "peticion", target_id: 5, request_state: "cancelada" } }),
    ]);
    const active = await screen.findByRole("list", { name: "Deseos activos" });
    expect(within(active).getByText("El titular rechazó tu petición: puedes convertirlo de nuevo.")).toBeInTheDocument();
    expect(within(active).getByText("Cancelaste tu petición: puedes convertirlo de nuevo.")).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Deseos convertidos" })).not.toBeInTheDocument();
  });

  it("shows the empty note and an error with a retry", async () => {
    renderPage([]);
    expect(await screen.findByText(/Aún no tienes deseos pendientes/)).toBeInTheDocument();
    cleanup();
    api.listWishlist.mockRejectedValue(new ApiError(500, "internal_error"));
    api.listPersonalCategories.mockResolvedValue(CATEGORIES);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <ListaDeDeseos />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("El servidor tuvo un problema");
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeInTheDocument();
  });
});

describe("ListaDeDeseos create, edit and delete", () => {
  it("opens the create form in a dialog from the header, validates and creates", async () => {
    api.createWishItem.mockResolvedValue(base);
    renderPage([]);
    await screen.findByText(/Aún no tienes deseos pendientes/);
    expect(screen.queryByLabelText("Nombre")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Nuevo deseo" }));
    expect(screen.getByRole("dialog", { name: "Nuevo deseo" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Agregar deseo" }));
    expect(await screen.findByText("Escribe qué quieres, por ejemplo Bolsa o Zapatos.")).toBeInTheDocument();
    expect(api.createWishItem).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: " Bolsa " } });
    fireEvent.change(screen.getByLabelText("Precio estimado (MXN)"), { target: { value: "499.90" } });
    fireEvent.change(screen.getByLabelText("Enlace de la tienda (opcional)"), { target: { value: "ftp://x.example" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar deseo" }));
    expect(await screen.findByText("Escribe un enlace que empiece con http:// o https://.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Enlace de la tienda (opcional)"), { target: { value: "https://tienda.example/b" } });
    fireEvent.change(screen.getByLabelText("Prioridad"), { target: { value: "alta" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar deseo" }));
    await waitFor(() => expect(api.createWishItem).toHaveBeenCalled());
    expect(api.createWishItem).toHaveBeenCalledWith({ name: "Bolsa", price: "499.90", category_id: 5, priority: "alta", store_url: "https://tienda.example/b" });
    expect(await screen.findByText("Bolsa se agregó a tu lista.")).toBeInTheDocument();
  });

  it("edits an item from its detail dialog", async () => {
    api.updateWishItem.mockResolvedValue(base);
    renderPage([base]);
    fireEvent.click(await screen.findByRole("button", { name: "Ver Bolsa" }));
    fireEvent.click(screen.getByRole("button", { name: "Editar Bolsa" }));
    expect(screen.getByRole("dialog", { name: "Editar Bolsa" })).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre")).toHaveValue("Bolsa");
    fireEvent.change(screen.getByLabelText("Precio estimado (MXN)"), { target: { value: "450" } });
    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));
    await waitFor(() => expect(api.updateWishItem).toHaveBeenCalled());
    expect(api.updateWishItem).toHaveBeenCalledWith(1, { name: "Bolsa", price: "450", category_id: 5, priority: "alta", store_url: "https://tienda.example/b", note: "roja" });
  });

  it("asks before deleting and says a converted target is untouched", async () => {
    api.deleteWishItem.mockResolvedValue(undefined);
    renderPage([item({ id: 2, name: "Zapatos", status: "convertida", conversion: { type: "gasto", target_id: 3, request_state: null } })]);
    fireEvent.click(await screen.findByRole("button", { name: "Ver Zapatos" }));
    fireEvent.click(screen.getByRole("button", { name: "Eliminar Zapatos" }));
    expect(screen.getByText("¿Eliminar este deseo? Lo que se creó con él no cambia.")).toBeInTheDocument();
    expect(api.deleteWishItem).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar" }));
    await waitFor(() => expect(api.deleteWishItem).toHaveBeenCalledWith(2));
  });
});

describe("ListaDeDeseos convert", () => {
  async function openDialog() {
    fireEvent.click(await screen.findByRole("button", { name: "Ver Bolsa" }));
    fireEvent.click(screen.getByRole("button", { name: "Convertir Bolsa" }));
    return screen.findByRole("dialog", { name: /Convertir/ });
  }

  it("offers the three ways with the price as default", async () => {
    renderPage([base]);
    const dialog = await openDialog();
    expect(within(dialog).getAllByRole("radio").map((r) => (r as HTMLInputElement).labels?.[0].textContent)).toEqual([
      expect.stringContaining("Gasto futuro"),
      expect.stringContaining("Ya lo compré"),
      expect.stringContaining("Pedir al titular"),
    ]);
    expect(within(dialog).getByLabelText("Monto a juntar (MXN)")).toHaveValue("499.90");
  });

  it("converts to a future expense and needs the due date", async () => {
    api.convertWishToFuture.mockResolvedValue(item({ status: "convertida", conversion: { type: "gasto_futuro", target_id: 9, request_state: null } }));
    renderPage([base]);
    const dialog = await openDialog();
    fireEvent.click(within(dialog).getByRole("button", { name: "Convertir" }));
    expect(await within(dialog).findByText("Elige la fecha de vencimiento.")).toBeInTheDocument();
    expect(api.convertWishToFuture).not.toHaveBeenCalled();
    fireEvent.change(within(dialog).getByLabelText("Fecha de vencimiento"), { target: { value: "2026-12-20" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Convertir" }));
    await waitFor(() => expect(api.convertWishToFuture).toHaveBeenCalledWith(1, { target_amount: "499.90", due_date: "2026-12-20" }));
    expect(await screen.findByText(/Bolsa se convirtió en un gasto futuro\./)).toBeInTheDocument();
  });

  it("converts to her Gasto with her categories", async () => {
    api.convertWishToExpense.mockResolvedValue(item({ status: "convertida", conversion: { type: "gasto", target_id: 3, request_state: null } }));
    renderPage([base]);
    const dialog = await openDialog();
    fireEvent.click(within(dialog).getByRole("radio", { name: /Ya lo compré/ }));
    expect(within(dialog).getByRole("option", { name: "Salud" })).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText("Categoría"), { target: { value: "6" } });
    fireEvent.change(within(dialog).getByLabelText("Monto (MXN)"), { target: { value: "450" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Convertir" }));
    await waitFor(() => expect(api.convertWishToExpense).toHaveBeenCalled());
    expect(api.convertWishToExpense).toHaveBeenCalledWith(1, { amount: "450", date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/), category_id: 6 });
  });

  it("asks the owner with one of the owner categories and shows a rate limit in Spanish", async () => {
    api.convertWishToRequest.mockRejectedValueOnce(new ApiError(429, "rate_limited"));
    api.convertWishToRequest.mockResolvedValueOnce(item({ status: "convertida", conversion: { type: "peticion", target_id: 4, request_state: "solicitada" } }));
    renderPage([base]);
    const dialog = await openDialog();
    fireEvent.click(within(dialog).getByRole("radio", { name: /Pedir al titular/ }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Enviar petición" }));
    expect(await within(dialog).findByText("Elige la categoría con la que se pedirá.")).toBeInTheDocument();
    expect(api.convertWishToRequest).not.toHaveBeenCalled();
    expect(await within(dialog).findByRole("option", { name: "Comida" })).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText("Categoría de la petición"), { target: { value: "Comida" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Enviar petición" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Enviaste demasiadas peticiones seguidas");
    fireEvent.click(within(dialog).getByRole("button", { name: "Enviar petición" }));
    await waitFor(() => expect(api.convertWishToRequest).toHaveBeenCalledTimes(2));
    expect(api.convertWishToRequest).toHaveBeenLastCalledWith(1, { amount: "499.90", date: expect.any(String), category: "Comida" });
    expect(await screen.findByText(/se envió como petición al titular/)).toBeInTheDocument();
  });

  it("shows the conflict of an item converted meanwhile", async () => {
    api.convertWishToExpense.mockRejectedValue(new ApiError(409, "already_converted"));
    renderPage([base]);
    const dialog = await openDialog();
    fireEvent.click(within(dialog).getByRole("radio", { name: /Ya lo compré/ }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Convertir" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Este deseo ya se convirtió.");
  });
});
