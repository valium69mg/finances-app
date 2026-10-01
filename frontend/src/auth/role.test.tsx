import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { tokenStore } from "../api/tokens";
import { canOpen, modules } from "../modules";
import { AuthProvider, useAuth } from "./AuthContext";

// The dashboard page is not under test here: it only has to show that the route rendered.
vi.mock("../pages/Dashboard", () => ({ Dashboard: () => <p>dashboard page</p> }));
vi.mock("../pages/Settings", () => ({ Settings: () => <p>settings page</p> }));
vi.mock("../pages/System", () => ({ System: () => <p>system page</p> }));
vi.mock("../pages/Income", () => ({ Income: () => <p>income page</p> }));
vi.mock("../api/expenseRequests", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/expenseRequests")>()),
  listExpenseRequests: vi.fn().mockResolvedValue([]),
}));
vi.mock("../api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/auth")>()),
  fetchMe: vi.fn().mockResolvedValue({ id: "u1", email: "someone@example.com", verified: true, role: "household" }),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => {
  localStorage.clear();
  // jsdom has no matchMedia, which the layout uses to close its drawer on wide screens.
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {} }));
});

function renderApp(path: string) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function signIn(role?: string) {
  localStorage.setItem("access_token", "a");
  localStorage.setItem("refresh_token", "r");
  if (role) localStorage.setItem("role", role);
}

describe("canOpen", () => {
  it("lets the owner open every module", () => {
    for (const m of modules) expect(canOpen("owner", m.path), m.path).toBe(true);
  });

  it("lets household open only the dashboard and the requests, so a new module is owner-only by default", () => {
    expect(modules.filter((m) => canOpen("household", m.path)).map((m) => m.path)).toEqual(["/", "/peticiones"]);
  });
});

describe("the session role", () => {
  it("is stored with the tokens and read back", () => {
    tokenStore.set({ access_token: "a", refresh_token: "r", role: "household" });
    expect(tokenStore.getRole()).toBe("household");
    tokenStore.clear();
    expect(tokenStore.getRole()).toBeNull();
  });

  it("ignores a role the app does not know", () => {
    tokenStore.set({ access_token: "a", refresh_token: "r", role: "admin" });
    expect(tokenStore.getRole()).toBeNull();
  });

  it("follows a role written by a refresh or /auth/me", () => {
    signIn("owner");
    function Probe() {
      return <p>role: {useAuth().role}</p>;
    }
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AuthProvider>
          <Probe />
        </AuthProvider>
      </QueryClientProvider>,
    );
    expect(screen.getByText("role: owner")).toBeInTheDocument();
    act(() => tokenStore.setRole("household"));
    expect(screen.getByText("role: household")).toBeInTheDocument();
  });

  it("treats a session stored before roles existed as the owner until the next refresh", () => {
    signIn();
    renderApp("/ingresos");
    expect(screen.getByText("income page")).toBeInTheDocument();
  });
});

describe("route guards", () => {
  it("household: every module but the dashboard and the requests redirects to /", () => {
    signIn("household");
    for (const path of modules.map((m) => m.path).filter((p) => p !== "/" && p !== "/peticiones")) {
      renderApp(path);
      expect(screen.getByText("dashboard page"), path).toBeInTheDocument();
      cleanup();
    }
  });

  it("household: an unknown path also lands on the dashboard", () => {
    signIn("household");
    renderApp("/no-such-page");
    expect(screen.getByText("dashboard page")).toBeInTheDocument();
  });

  it("household: the navigation lists only the dashboard and the requests", () => {
    signIn("household");
    renderApp("/");
    const nav = screen.getAllByRole("navigation", { name: "Módulos" })[0];
    expect(Array.from(nav.querySelectorAll("a")).map((a) => a.textContent)).toEqual(["Panel", "Peticiones"]);
  });

  it("owner: modules and the navigation are unchanged", () => {
    signIn("owner");
    renderApp("/sistema");
    expect(screen.getByText("system page")).toBeInTheDocument();
    const nav = screen.getAllByRole("navigation", { name: "Módulos" })[0];
    expect(nav.querySelectorAll("a")).toHaveLength(modules.length);
  });
});
