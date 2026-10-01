import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { UserAccount } from "../../api/users";
import { describeUsersError } from "./errors";
import { validateNewUser } from "./form";
import { UsersSection } from "./UsersSection";

const api = vi.hoisted(() => ({
  listUsers: vi.fn(),
  createUser: vi.fn(),
  inviteUser: vi.fn(),
  deactivateUser: vi.fn(),
  activateUser: vi.fn(),
}));
const authApi = vi.hoisted(() => ({ fetchMe: vi.fn() }));
vi.mock("../../api/users", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/users")>()), ...api }));
vi.mock("../../api/auth", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/auth")>()), ...authApi }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const OWNER: UserAccount = { id: "u-owner", email: "owner@example.com", role: "owner", active: true, verified: true, created_at: "2026-09-01T00:00:00Z" };
const PENDING: UserAccount = { id: "u-her", email: "her@example.com", role: "household", active: true, verified: false, created_at: "2026-09-02T00:00:00Z" };
const VERIFIED: UserAccount = { id: "u-done", email: "done@example.com", role: "household", active: true, verified: true, created_at: "2026-09-03T00:00:00Z" };
const OFF: UserAccount = { id: "u-off", email: "off@example.com", role: "household", active: false, verified: true, created_at: "2026-09-04T00:00:00Z" };

function open(users: UserAccount[] = [OWNER, PENDING, VERIFIED, OFF]) {
  api.listUsers.mockResolvedValue(users);
  authApi.fetchMe.mockResolvedValue({ id: OWNER.id, email: OWNER.email, verified: true, role: "owner" });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <UsersSection />
    </QueryClientProvider>,
  );
}

const rowOf = async (email: string) => within(await screen.findByRole("list", { name: "Usuarios" })).getAllByRole("listitem").find((li) => li.textContent?.includes(email)) as HTMLElement;

describe("users form helpers", () => {
  it("validates the email shape", () => {
    expect(validateNewUser("her@example.com")).toEqual({});
    expect(validateNewUser("  her@example.com ")).toEqual({});
    expect(validateNewUser("").email).toContain("Escribe");
    for (const bad of ["her", "her@", "@example.com", "her @example.com", "her@example"]) expect(validateNewUser(bad).email, bad).toContain("válido");
  });

  it("maps every API failure to Spanish copy", () => {
    const cases: [number, string, RegExp][] = [
      [409, "email_taken", /Ya existe/],
      [400, "invalid_email", /válido/],
      [409, "cannot_change_self", /propia cuenta/],
      [409, "already_verified", /ya activó/],
      [409, "user_inactive", /desactivada/],
      [429, "rate_limited", /demasiadas invitaciones/i],
      [502, "email_failed", /No se pudo enviar/],
      [404, "not_found", /ya no existe/],
      [403, "forbidden", /permiso/],
    ];
    for (const [status, code, text] of cases) expect(describeUsersError(new ApiError(status, code)), code).toMatch(text);
    expect(describeUsersError(new TypeError("Failed to fetch"))).toContain("conectar");
  });
});

describe("UsersSection", () => {
  it("lists email, role, state and verification, and gives the owner no actions on their own row", async () => {
    open();
    const owner = await rowOf(OWNER.email);
    expect(owner).toHaveTextContent("Titular");
    expect(owner).toHaveTextContent("Activo");
    expect(owner).toHaveTextContent("Tu cuenta");
    expect(within(owner).queryByRole("button")).toBeNull();

    const her = await rowOf(PENDING.email);
    expect(her).toHaveTextContent("Familiar");
    expect(her).toHaveTextContent("Sin verificar");
    expect(within(her).getByRole("button", { name: /Enviar invitación a her@example.com/ })).toBeInTheDocument();
    expect(within(her).getByRole("button", { name: /Desactivar a her@example.com/ })).toBeInTheDocument();

    // A verified account has nothing to be invited to; an inactive one offers Activar only.
    const done = await rowOf(VERIFIED.email);
    expect(done).toHaveTextContent("Verificado");
    expect(within(done).queryByRole("button", { name: /invitación/ })).toBeNull();
    const off = await rowOf(OFF.email);
    expect(off).toHaveTextContent("Inactivo");
    expect(within(off).getByRole("button", { name: /Activar a off@example.com/ })).toBeInTheDocument();
    expect(within(off).queryByRole("button", { name: /Desactivar/ })).toBeNull();
  });

  it("creates a household account behind the collapsed form and says the invitation is next", async () => {
    api.createUser.mockResolvedValue({ ...PENDING, id: "u-new", email: "new@example.com" });
    open();
    expect(screen.queryByRole("form", { name: "Nuevo usuario" })).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Nuevo usuario" }));
    fireEvent.change(screen.getByLabelText("Correo electrónico"), { target: { value: "  new@example.com " } });
    fireEvent.click(screen.getByRole("button", { name: "Crear usuario" }));
    await waitFor(() => expect(api.createUser).toHaveBeenCalledWith({ email: "new@example.com", role: "household" }));
    expect(await screen.findByRole("status")).toHaveTextContent("new@example.com se creó sin contraseña");
  });

  it("does not call the API for an invalid email", async () => {
    open();
    fireEvent.click(await screen.findByRole("button", { name: "Nuevo usuario" }));
    fireEvent.change(screen.getByLabelText("Correo electrónico"), { target: { value: "nope" } });
    fireEvent.click(screen.getByRole("button", { name: "Crear usuario" }));
    expect(await screen.findByText("Escribe un correo electrónico válido.")).toBeInTheDocument();
    expect(api.createUser).not.toHaveBeenCalled();
  });

  it("shows the server's duplicate email error", async () => {
    api.createUser.mockRejectedValue(new ApiError(409, "email_taken"));
    open();
    fireEvent.click(await screen.findByRole("button", { name: "Nuevo usuario" }));
    fireEvent.change(screen.getByLabelText("Correo electrónico"), { target: { value: "her@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Crear usuario" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Ya existe una cuenta con ese correo.");
  });

  it("asks for confirmation before inviting, then offers Reenviar", async () => {
    api.inviteUser.mockResolvedValue(undefined);
    open();
    fireEvent.click(within(await rowOf(PENDING.email)).getByRole("button", { name: /Enviar invitación/ }));
    const dialog = await screen.findByRole("dialog", { name: "Enviar invitación" });
    expect(dialog).toHaveTextContent("1 hora");
    expect(api.inviteUser).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(api.inviteUser).toHaveBeenCalledWith("u-her"));
    expect(await screen.findByRole("status")).toHaveTextContent("Invitación enviada a her@example.com");
    expect(within(await rowOf(PENDING.email)).getByRole("button", { name: /Reenviar invitación/ })).toBeInTheDocument();
  });

  it("cancelling a confirmation changes nothing", async () => {
    open();
    fireEvent.click(within(await rowOf(PENDING.email)).getByRole("button", { name: /Desactivar/ }));
    fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancelar" }));
    expect(api.deactivateUser).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("deactivates and activates after confirming", async () => {
    api.deactivateUser.mockResolvedValue(undefined);
    api.activateUser.mockResolvedValue(undefined);
    open();
    fireEvent.click(within(await rowOf(PENDING.email)).getByRole("button", { name: /Desactivar/ }));
    let dialog = await screen.findByRole("dialog", { name: "Desactivar usuario" });
    expect(dialog).toHaveTextContent("se cerrarán sus sesiones");
    fireEvent.click(within(dialog).getByRole("button", { name: "Desactivar" }));
    await waitFor(() => expect(api.deactivateUser).toHaveBeenCalledWith("u-her"));

    fireEvent.click(within(await rowOf(OFF.email)).getByRole("button", { name: /Activar/ }));
    dialog = await screen.findByRole("dialog", { name: "Activar usuario" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Activar" }));
    await waitFor(() => expect(api.activateUser).toHaveBeenCalledWith("u-off"));
  });

  it("keeps the dialog open and explains a failed action", async () => {
    api.inviteUser.mockRejectedValue(new ApiError(429, "rate_limited"));
    open();
    fireEvent.click(within(await rowOf(PENDING.email)).getByRole("button", { name: /Enviar invitación/ }));
    fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Enviar" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("demasiadas invitaciones");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("does not crash when the list is refused (403) and offers a retry", async () => {
    api.listUsers.mockRejectedValue(new ApiError(403, "forbidden"));
    authApi.fetchMe.mockResolvedValue({ id: "x", email: "x@example.com", verified: true, role: "household" });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <UsersSection />
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("no tiene permiso");
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeInTheDocument();
  });
});
