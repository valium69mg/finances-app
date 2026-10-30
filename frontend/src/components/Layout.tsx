import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigationType } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, CircleUser, LogOut, Wallet, X } from "lucide-react";
import { fetchMe } from "../api/auth";
import { tokenStore } from "../api/tokens";
import { useAuth } from "../auth/AuthContext";
import { modulesFor } from "../navigation";
import { BottomDock } from "./BottomDock";
import { LogoutDialog } from "./LogoutDialog";
import { MoreSheet } from "./MoreSheet";
import { PendingRequestsBadge } from "./PendingRequestsBadge";
import { QuickExpense, type QuickExpenseResult } from "./QuickExpense";
import { ThemeSelector } from "./ThemeSelector";
import { BudgetFeedbackNote } from "../pages/expenses/BudgetFeedbackNote";

const DESKTOP_QUERY = "(min-width: 1024px)";

function ModuleNav() {
  const { role } = useAuth();
  return (
    <nav aria-label="Módulos" className="min-h-0 flex-1 overflow-y-auto p-3">
      <ul className="space-y-1">
        {modulesFor(role).map((m) => {
          const Icon = m.icon;
          return (
            <li key={m.path}>
              <NavLink
                to={m.path}
                end={m.path === "/"}
                className={({ isActive }) =>
                  `focus-ring flex min-h-11 items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors duration-200 ${
                    isActive ? "bg-primary/10 font-semibold text-primary shadow-[inset_3px_0_0_rgb(var(--color-primary))]" : "text-muted hover:bg-primary/5 hover:text-foreground"
                  }`
                }
              >
                <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
                <span className="min-w-0 break-words">{m.label}</span>
                {m.path === "/peticiones" && role === "owner" && <PendingRequestsBadge />}
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

type Overlay = "quick" | "more" | null;

/** How long the "gasto guardado" notice stays up (the owner's carries the budget feedback, so it gets longer). */
const NOTICE_MS = 6000;

export function Layout() {
  const { role } = useAuth();
  const location = useLocation();
  const navigationType = useNavigationType();
  const [confirmingLogout, setConfirmingLogout] = useState(false);
  const [overlay, setOverlay] = useState<Overlay>(null);
  const [notice, setNotice] = useState<{ result: QuickExpenseResult } | null>(null);
  const mainRef = useRef<HTMLElement>(null);
  const shownPath = useRef(location.pathname);
  const me = useQuery({ queryKey: ["me"], queryFn: fetchMe, retry: false, staleTime: 5 * 60_000 });

  // The server's word on the role wins over what the session stored (e.g. a role changed since the last refresh).
  const serverRole = me.data?.role;
  useEffect(() => {
    if (serverRole) tokenStore.setRole(serverRole);
  }, [serverRole]);

  // After a client-side navigation to another page: land at the top (a new page must not inherit the previous scroll
  // offset, which hid the title under the sticky header) and move focus to the content without scrolling. Back and
  // forward (POP) keep whatever the browser restores. Comparing the path (instead of a first-render flag) also skips
  // the first load when StrictMode runs the effect twice in development.
  useEffect(() => {
    if (shownPath.current === location.pathname) return;
    shownPath.current = location.pathname;
    if (navigationType !== "POP") window.scrollTo(0, 0);
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname, navigationType]);

  // Dock overlays: close them when the viewport grows into the sidebar layout (the dock is gone there).
  useEffect(() => {
    const mq = window.matchMedia(DESKTOP_QUERY);
    const onChange = () => {
      if (mq.matches) setOverlay(null);
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  useEffect(() => {
    if (!notice) return;
    const t = window.setTimeout(() => setNotice(null), NOTICE_MS);
    return () => window.clearTimeout(t);
  }, [notice]);

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
        <div className="shrink-0 border-t border-border p-3">
          <ThemeSelector />
        </div>
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 border-b border-border bg-surface pt-[env(safe-area-inset-top)]">
          <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-3 px-4 sm:px-6">
            <div className="lg:hidden">
              <Brand />
            </div>
            <div className="ml-auto hidden items-center gap-3 lg:flex">
              {me.data && (
                <span className="flex min-w-0 items-center gap-2 text-sm text-muted">
                  <CircleUser className="h-5 w-5 shrink-0" aria-hidden="true" />
                  <span className="max-w-[16rem] truncate">{me.data.email}</span>
                </span>
              )}
              <div className="border-l border-border pl-3">
                <button
                  type="button"
                  onClick={() => setConfirmingLogout(true)}
                  aria-haspopup="dialog"
                  className="focus-ring inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-medium text-muted transition-colors duration-200 hover:bg-destructive/10 hover:text-destructive"
                >
                  <LogOut className="h-5 w-5" aria-hidden="true" />
                  Cerrar sesión
                </button>
              </div>
            </div>
          </div>
        </header>

        <main
          id="contenido"
          ref={mainRef}
          tabIndex={-1}
          className="mx-auto max-w-6xl px-4 pb-[calc(6.5rem+env(safe-area-inset-bottom))] pt-8 outline-none sm:px-6 lg:pb-[calc(2rem+env(safe-area-inset-bottom))]"
        >
          <Outlet />
        </main>
      </div>

      <BottomDock role={role} onQuickExpense={() => setOverlay("quick")} onMore={() => setOverlay("more")} quickOpen={overlay === "quick"} moreOpen={overlay === "more"} />

      <div className="pointer-events-none fixed inset-x-0 bottom-[calc(5.5rem+env(safe-area-inset-bottom))] z-30 flex justify-center px-4 lg:hidden">
        {notice && (
          <div data-testid="quick-expense-notice" className="pointer-events-auto flex max-w-md items-start gap-2 rounded-xl border border-border bg-surface p-3 shadow-lg">
            <div className="min-w-0 flex-1" aria-live="polite">
              {notice.result ? (
                <BudgetFeedbackNote result={notice.result} />
              ) : (
                <p role="status" className="flex items-center gap-1.5 text-sm">
                  <CheckCircle2 className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                  Gasto guardado.
                </p>
              )}
            </div>
            <button
              type="button"
              onClick={() => setNotice(null)}
              aria-label="Cerrar aviso"
              className="focus-ring -m-1 flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-muted hover:bg-primary/10 hover:text-foreground"
            >
              <X className="h-5 w-5" aria-hidden="true" />
            </button>
          </div>
        )}
      </div>

      {overlay === "quick" && <QuickExpense role={role} onClose={() => setOverlay(null)} onSaved={(result) => setNotice({ result })} />}
      {confirmingLogout && <LogoutDialog onClose={() => setConfirmingLogout(false)} />}
      {overlay === "more" && <MoreSheet email={me.data?.email} onClose={() => setOverlay(null)} />}
    </div>
  );
}
