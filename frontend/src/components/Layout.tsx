import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { CircleUser, LogOut, Menu, Wallet, X } from "lucide-react";
import { fetchMe } from "../api/auth";
import { useAuth } from "../auth/AuthContext";
import { modules } from "../modules";

const DESKTOP_QUERY = "(min-width: 1024px)";

function ModuleNav({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <nav aria-label="Módulos" className="min-h-0 flex-1 overflow-y-auto p-3">
      <ul className="space-y-1">
        {modules.map((m) => {
          const Icon = m.icon;
          return (
            <li key={m.path}>
              <NavLink
                to={m.path}
                end={m.path === "/"}
                onClick={onNavigate}
                className={({ isActive }) =>
                  `focus-ring flex min-h-11 items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors duration-200 ${
                    isActive ? "bg-primary/10 font-semibold text-primary" : "text-muted hover:bg-primary/5 hover:text-foreground"
                  }`
                }
              >
                <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
                <span className="min-w-0 break-words">{m.label}</span>
              </NavLink>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function Brand() {
  return (
    <span className="flex items-center gap-2 text-lg font-semibold tracking-tight text-primary">
      <Wallet className="h-6 w-6" aria-hidden="true" />
      Finanzas
    </span>
  );
}

export function Layout() {
  const { logout } = useAuth();
  const location = useLocation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const mainRef = useRef<HTMLElement>(null);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const shownPath = useRef(location.pathname);
  const me = useQuery({ queryKey: ["me"], queryFn: fetchMe, retry: false, staleTime: 5 * 60_000 });

  const closeDrawer = useCallback(() => {
    setDrawerOpen(false);
    menuButtonRef.current?.focus();
  }, []);

  // Move focus to the page content after client-side navigation. Comparing the path (instead of a
  // first-render flag) also skips the first load when StrictMode runs the effect twice in development.
  useEffect(() => {
    if (shownPath.current === location.pathname) return;
    shownPath.current = location.pathname;
    mainRef.current?.focus();
  }, [location.pathname]);

  // Drawer: close it when the viewport grows into the sidebar layout.
  useEffect(() => {
    const mq = window.matchMedia(DESKTOP_QUERY);
    const onChange = () => {
      if (mq.matches) setDrawerOpen(false);
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  // Drawer: move focus in on open and lock background scroll.
  useEffect(() => {
    if (!drawerOpen) return;
    drawerRef.current?.querySelector<HTMLElement>("button, a")?.focus();
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [drawerOpen]);

  function onDrawerKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      e.stopPropagation();
      closeDrawer();
      return;
    }
    if (e.key !== "Tab") return;
    const focusable = drawerRef.current?.querySelectorAll<HTMLElement>("button, a[href]");
    if (!focusable || focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <a
        href="#contenido"
        onClick={(e) => {
          e.preventDefault();
          mainRef.current?.focus();
        }}
        className="focus-ring sr-only rounded-lg bg-primary px-4 py-2 font-medium text-on-primary focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50"
      >
        Saltar al contenido
      </a>

      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r border-border bg-surface lg:flex">
        <div className="flex h-16 shrink-0 items-center border-b border-border px-5">
          <Brand />
        </div>
        <ModuleNav />
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 border-b border-border bg-surface pt-[env(safe-area-inset-top)]">
          <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-3 px-4 sm:px-6">
            <div className="flex items-center gap-2 lg:hidden">
              <button
                ref={menuButtonRef}
                type="button"
                onClick={() => setDrawerOpen(true)}
                aria-label="Abrir menú"
                aria-haspopup="dialog"
                aria-expanded={drawerOpen}
                className="focus-ring -ml-2 flex h-11 w-11 items-center justify-center rounded-lg text-foreground transition-colors duration-200 hover:bg-primary/10"
              >
                <Menu className="h-6 w-6" aria-hidden="true" />
              </button>
              <Brand />
            </div>
            <div className="ml-auto flex items-center gap-3">
              {me.data && (
                <span className="hidden min-w-0 items-center gap-2 text-sm text-muted sm:flex">
                  <CircleUser className="h-5 w-5 shrink-0" aria-hidden="true" />
                  <span className="max-w-[16rem] truncate">{me.data.email}</span>
                </span>
              )}
              <div className="border-l border-border pl-3">
                <button
                  type="button"
                  onClick={() => void logout()}
                  className="focus-ring inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-medium text-muted transition-colors duration-200 hover:bg-destructive/10 hover:text-destructive"
                >
                  <LogOut className="h-5 w-5" aria-hidden="true" />
                  Cerrar sesión
                </button>
              </div>
            </div>
          </div>
        </header>

        <main id="contenido" ref={mainRef} tabIndex={-1} className="mx-auto max-w-6xl px-4 pb-[calc(2rem+env(safe-area-inset-bottom))] pt-8 outline-none sm:px-6">
          <Outlet />
        </main>
      </div>

      {drawerOpen && (
        <div
          ref={drawerRef}
          role="dialog"
          aria-modal="true"
          aria-label="Menú de navegación"
          onKeyDown={onDrawerKeyDown}
          className="fixed inset-0 z-40 lg:hidden"
        >
          <div className="absolute inset-0 animate-fade-in bg-foreground/50" onClick={closeDrawer} aria-hidden="true" />
          <div className="absolute inset-y-0 left-0 flex w-72 max-w-[85vw] animate-drawer-in flex-col border-r border-border bg-surface pb-[env(safe-area-inset-bottom)] pt-[env(safe-area-inset-top)] shadow-xl">
            <div className="flex h-16 shrink-0 items-center justify-between border-b border-border pl-5 pr-3">
              <Brand />
              <button
                type="button"
                onClick={closeDrawer}
                aria-label="Cerrar menú"
                className="focus-ring flex h-11 w-11 items-center justify-center rounded-lg text-muted transition-colors duration-200 hover:bg-primary/10 hover:text-foreground"
              >
                <X className="h-6 w-6" aria-hidden="true" />
              </button>
            </div>
            <ModuleNav onNavigate={() => setDrawerOpen(false)} />
          </div>
        </div>
      )}
    </div>
  );
}
