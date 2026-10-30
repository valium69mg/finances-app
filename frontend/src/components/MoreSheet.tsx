import { useState } from "react";
import { NavLink } from "react-router-dom";
import { CircleUser, LogOut } from "lucide-react";
import { useAuth } from "../auth/AuthContext";
import { moreModules } from "../navigation";
import { LogoutDialog } from "./LogoutDialog";
import { Modal } from "./Modal";
import { PendingRequestsBadge } from "./PendingRequestsBadge";
import { ThemeSelector } from "./ThemeSelector";

/**
 * "Más" bottom sheet of the mobile dock: every module the dock does not show, the theme selector and the session
 * (email and "Cerrar sesión"), which the slimmer mobile top bar no longer carries.
 */
export function MoreSheet({ email, onClose }: { email?: string; onClose: () => void }) {
  const { role } = useAuth();
  const [confirmingLogout, setConfirmingLogout] = useState(false);
  return (
    <>
    <Modal title="Más" onClose={onClose} sheet>
      <nav aria-label="Más módulos">
        <ul className="grid grid-cols-3 gap-2">
          {moreModules(role).map((m) => {
            const Icon = m.icon;
            return (
              <li key={m.path}>
                <NavLink
                  to={m.path}
                  onClick={onClose}
                  className={({ isActive }) =>
                    `focus-ring relative flex min-h-20 flex-col items-center justify-center gap-1.5 rounded-lg border px-1.5 py-2 text-center text-xs font-medium leading-tight transition-colors duration-200 ${
                      isActive ? "border-primary bg-primary/10 font-semibold text-primary" : "border-border text-foreground hover:bg-primary/5"
                    }`
                  }
                >
                  <Icon className="h-6 w-6 shrink-0" aria-hidden="true" />
                  <span className="break-words">{m.label}</span>
                  {m.path === "/peticiones" && role === "owner" && <PendingRequestsBadge className="absolute right-1.5 top-1.5" />}
                </NavLink>
              </li>
            );
          })}
        </ul>
      </nav>

      <div className="mt-5 border-t border-border pt-5">
        <ThemeSelector />
      </div>

      <div className="mt-5 flex items-center justify-between gap-3 border-t border-border pt-4">
        {email ? (
          <p className="flex min-w-0 items-center gap-2 text-sm text-muted">
            <CircleUser className="h-5 w-5 shrink-0" aria-hidden="true" />
            <span className="min-w-0 truncate">{email}</span>
          </p>
        ) : (
          <span />
        )}
        <button
          type="button"
          onClick={() => setConfirmingLogout(true)}
          aria-haspopup="dialog"
          className="focus-ring inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium text-muted transition-colors duration-200 hover:bg-destructive/10 hover:text-destructive focus-visible:text-destructive"
        >
          <LogOut className="h-4 w-4" aria-hidden="true" />
          Cerrar sesión
        </button>
      </div>
    </Modal>
    {confirmingLogout && <LogoutDialog onClose={() => setConfirmingLogout(false)} />}
    </>
  );
}
