import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WishItem } from "../../api/wishlist";
import type { UserAccount } from "../../api/users";
import { Hogar } from "../Hogar";
import { LIST } from "../futureexpenses/fixtures";

const api = vi.hoisted(() => ({
  listUsers: vi.fn(),
  getPersonalDashboard: vi.fn(),
  getPersonalPeriod: vi.fn(),
  listPersonalCategories: vi.fn(),
  listPersonalMovements: vi.fn(),
  listPersonalFutureExpenses: vi.fn(),
  listWishlist: vi.fn(),
}));
vi.mock("../../api/users", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/users")>()), listUsers: api.listUsers }));
vi.mock("../../api/personal", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/personal")>()), ...api }));
vi.mock("../../api/wishlist", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/wishlist")>()), listWishlist: api.listWishlist }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const user = (id: string, email: string, patch: Partial<UserAccount> = {}): UserAccount => ({
  id,
  email,
  role: "household",
  active: true,
  verified: true,
  created_at: "2026-09-01T12:00:00Z",
  ...patch,
});
const OWNER = user("o1", "owner@example.com", { role: "owner" });
const ANA = user("u1", "ana@example.com");
const LUZ = user("u2", "luz@example.com");

const DASHBOARD = {
  month: "2026-10",
  period_start: "2026-09-15",
  period_end: "2026-10-14",
  income: "10000.00",
  expenses: "1300.50",
  savings: "1500.00",
  available: "7199.50",
  categories: [{ category_id: 5, category: "Ropa", spent: "1300.50" }],
};
const CATEGORIES = [
  { id: 5, kind: "Gasto", name: "Ropa", fixed: false },
  { id: 6, kind: "Gasto", name: "Salud", fixed: false },
];
const MOVEMENT = {
  id: 7,
  kind: "Gasto",
  category_id: 5,
  category: "Ropa",
  amount: "199.90",
  description: "Blusa",
  date: "2026-10-04",
  future_expense_id: null,
  created_at: "2026-10-01T12:00:00Z",
  updated_at: "2026-10-01T12:00:00Z",
};
const WISH: WishItem = {
  id: 1,
  name: "Bolsa",
  price: "499.90",
  category_id: 5,
  category: "Ropa",
  store_url: null,
  priority: "alta",
  note: null,
  status: "activa",
  conversion: null,
  created_at: "2026-10-01T12:00:00Z",
  updated_at: "2026-10-01T12:00:00Z",
};

function Where() {
  const loc = useLocation();
  return <p data-testid="where">{loc.pathname + loc.search}</p>;
}

function renderHogar(url: string, users: UserAccount[] = [OWNER, ANA], setup?: () => void) {
  api.listUsers.mockResolvedValue(users);
  api.getPersonalDashboard.mockResolvedValue(DASHBOARD);
  api.getPersonalPeriod.mockResolvedValue({ month: "2026-10", period_start: "2026-09-15", period_end: "2026-10-14" });
  api.listPersonalCategories.mockResolvedValue(CATEGORIES);
  api.listPersonalMovements.mockResolvedValue([MOVEMENT]);
  api.listPersonalFutureExpenses.mockResolvedValue(LIST);
  api.listWishlist.mockResolvedValue([WISH, { ...WISH, id: 2, name: "Zapatos", status: "convertida", conversion: { type: "gasto_futuro", target_id: 9, request_state: null } }]);
  setup?.();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <Hogar />
        <Where />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** No control that writes is rendered, in any section. */
function expectNoWriteControls() {
  const writes = /nuevo|nueva|editar|eliminar|convertir|agregar|asignar|marcar pagado|administrar|guardar|aportaci[oó]n|retiro/i;
  for (const control of [...screen.queryAllByRole("button"), ...screen.queryAllByRole("link")]) {
    const name = control.getAttribute("aria-label") ?? control.textContent ?? "";
    if (/^(Resumen|Ingresos|Gastos|Ahorros|Gastos futuros|Lista de deseos)$/.test(name)) continue;
    expect(name, `control "${name}"`).not.toMatch(writes);
  }
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(document.querySelector("form")).toBeNull();
}

describe("Hogar", () => {
  it("selects the only household user by itself and titles the page with the email", async () => {
    renderHogar("/hogar");
    expect(await screen.findByRole("heading", { level: 1, name: "Finanzas de ana@example.com" })).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Familiar" })).not.toBeInTheDocument();
    await waitFor(() => expect(api.getPersonalDashboard).toHaveBeenCalledWith("", "u1"));
  });

  it("asks which person with several users and keeps the choice in the URL", async () => {
    renderHogar("/hogar", [OWNER, ANA, LUZ, user("u3", "inactiva@example.com", { active: false })]);
    const select = await screen.findByRole("combobox", { name: "Familiar" });
    expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Elige a un familiar", "ana@example.com", "luz@example.com"]);
    expect(screen.getByRole("heading", { level: 1, name: "Hogar" })).toBeInTheDocument();
    expect(api.getPersonalDashboard).not.toHaveBeenCalled();

    fireEvent.change(select, { target: { value: "u2" } });
    expect(await screen.findByRole("heading", { level: 1, name: "Finanzas de luz@example.com" })).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("/hogar?usuario=u2");
    await waitFor(() => expect(api.getPersonalDashboard).toHaveBeenCalledWith("", "u2"));
  });

  it("reads the person from ?usuario= and ignores an id that is not an active household user", async () => {
    renderHogar("/hogar?usuario=u2", [OWNER, ANA, LUZ]);
    expect(await screen.findByRole("heading", { level: 1, name: "Finanzas de luz@example.com" })).toBeInTheDocument();
    cleanup();
    renderHogar("/hogar?usuario=o1", [OWNER, ANA, LUZ]);
    expect(await screen.findByRole("combobox", { name: "Familiar" })).toHaveValue("");
  });

  it("explains the empty state and links to Configuración", async () => {
    renderHogar("/hogar", [OWNER, user("u9", "off@example.com", { active: false })]);
    expect(await screen.findByText("Aún no hay familiares con cuenta activa.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Configuración > Usuarios/ })).toHaveAttribute("href", "/configuracion");
    expect(api.getPersonalDashboard).not.toHaveBeenCalled();
  });

  it("Resumen shows her cards and categories with links that stay inside Hogar and carry the month", async () => {
    renderHogar("/hogar?mes=2026-10");
    const cards = within(await waitFor(() => {
      const dl = document.querySelector("dl");
      if (!dl) throw new Error("cards not rendered yet");
      return dl as HTMLElement;
    }));
    expect(cards.getByRole("link", { name: /Ingresos/ })).toHaveAttribute("href", "/hogar?usuario=u1&seccion=ingresos&mes=2026-10");
    expect(cards.getByRole("link", { name: /Gastos/ })).toHaveAttribute("href", "/hogar?usuario=u1&seccion=gastos&mes=2026-10");
    expect(cards.getByRole("link", { name: /Ahorros/ })).toHaveAttribute("href", "/hogar?usuario=u1&seccion=ahorros&mes=2026-10");
    expect(cards.queryByRole("link", { name: /Disponible/ })).not.toBeInTheDocument();
    const row = within(await screen.findByTestId("category-spend")).getByRole("link");
    expect(row).toHaveAttribute("href", "/hogar?usuario=u1&seccion=gastos&mes=2026-10&categoria=5");
    for (const a of document.querySelectorAll<HTMLAnchorElement>("a")) {
      const href = a.getAttribute("href") ?? "";
      expect(href.startsWith("/hogar") || href === "/configuracion", href).toBe(true);
    }
    expect(screen.queryByText("Presupuesto por categoría")).not.toBeInTheDocument();
    expectNoWriteControls();
    expect(api.getPersonalDashboard).toHaveBeenCalledWith("2026-10", "u1");
  });

  it("the section tabs keep the person and the month and mark the current one", async () => {
    renderHogar("/hogar?usuario=u1&seccion=gastos&mes=2026-10&categoria=5");
    const nav = await screen.findByRole("navigation", { name: "Secciones de las finanzas" });
    expect(within(nav).getAllByRole("link").map((a) => a.textContent)).toEqual(["Resumen", "Ingresos", "Gastos", "Ahorros", "Gastos futuros", "Lista de deseos"]);
    expect(within(nav).getByRole("link", { name: "Gastos" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Ahorros" })).toHaveAttribute("href", "/hogar?usuario=u1&seccion=ahorros&mes=2026-10");
    expect(within(nav).getByRole("link", { name: "Resumen" })).toHaveAttribute("href", "/hogar?usuario=u1&mes=2026-10");
  });

  it("Gastos lists her expenses read-only with the category chip filter", async () => {
    renderHogar("/hogar?usuario=u1&seccion=gastos&mes=2026-10&categoria=5");
    expect(await screen.findByText("Blusa")).toBeInTheDocument();
    expect(await screen.findByText("Categoría: Ropa")).toBeInTheDocument();
    expect(api.listPersonalMovements).toHaveBeenCalledWith({ month: "2026-10", kind: "Gasto", categoryId: 5, userId: "u1" });
    expect(api.listPersonalCategories).toHaveBeenCalledWith("Gasto", "u1");
    expect(api.getPersonalPeriod).toHaveBeenCalledWith("2026-10", "u1");
    expect(screen.getByRole("button", { name: "Quitar filtro de categoría Ropa" })).toBeInTheDocument();
    expectNoWriteControls();
  });

  it.each([
    ["ingresos", "Ingreso"],
    ["ahorros", "Ahorro"],
  ])("%s lists the %s rows of the person without write controls", async (seccion, kind) => {
    renderHogar(`/hogar?usuario=u1&seccion=${seccion}`);
    expect(await screen.findByText("Blusa")).toBeInTheDocument();
    expect(api.listPersonalMovements).toHaveBeenCalledWith({ month: "", kind, categoryId: undefined, userId: "u1" });
    expectNoWriteControls();
  });

  it("Gastos futuros shows the free balance, progress, suggested monthly and the paid history only", async () => {
    renderHogar("/hogar?usuario=u1&seccion=gastos-futuros");
    expect(await screen.findByText("Saldo libre")).toBeInTheDocument();
    expect(api.listPersonalFutureExpenses).toHaveBeenCalledWith("u1");
    const active = screen.getByRole("list", { name: "Gastos futuros activos" });
    expect(within(active).getByText("Laptop")).toBeInTheDocument();
    expect(within(active).getByRole("progressbar", { name: "Ahorro para Laptop" })).toBeInTheDocument();
    expect(within(active).getByText(/Aparta \$1,500\.00 al mes/)).toBeInTheDocument();
    expect(within(screen.getByRole("list", { name: "Gastos futuros pagados" })).getByText("Teléfono")).toBeInTheDocument();
    expectNoWriteControls();
  });

  it("Lista de deseos shows active and converted items with what each became, inside Hogar", async () => {
    renderHogar("/hogar?usuario=u1&seccion=deseos");
    const active = await screen.findByRole("list", { name: "Deseos activos" });
    expect(within(active).getByText("Bolsa")).toBeInTheDocument();
    const converted = screen.getByRole("list", { name: "Deseos convertidos" });
    expect(within(converted).getByText("Se convirtió en un gasto futuro.")).toBeInTheDocument();
    expect(within(converted).getByRole("link", { name: "Ver en Gastos futuros" })).toHaveAttribute("href", "/hogar?usuario=u1&seccion=gastos-futuros");
    expect(api.listWishlist).toHaveBeenCalledWith("u1");
    expect(api.listPersonalCategories).not.toHaveBeenCalled();
    expectNoWriteControls();
  });

  it("a request conversion has no link out of Hogar", async () => {
    renderHogar("/hogar?usuario=u1&seccion=deseos", [OWNER, ANA], () =>
      api.listWishlist.mockResolvedValue([{ ...WISH, status: "convertida", conversion: { type: "peticion", target_id: 4, request_state: "solicitada" } }]),
    );
    const converted = await screen.findByRole("list", { name: "Deseos convertidos" });
    expect(within(converted).queryByRole("link")).not.toBeInTheDocument();
  });

  it("surfaces a failed read with a retry", async () => {
    renderHogar("/hogar?usuario=u1&seccion=gastos-futuros", [OWNER, ANA], () => api.listPersonalFutureExpenses.mockRejectedValue(new Error("boom")));
    expect(await screen.findByRole("button", { name: "Reintentar" })).toBeInTheDocument();
  });
});
