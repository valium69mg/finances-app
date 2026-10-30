import type { MouseEvent } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { Ellipsis, Plus } from "lucide-react";
import { dockItems, type DockItem } from "../navigation";

interface Props {
  role: "owner" | "household";
  onQuickExpense: () => void;
  onMore: () => void;
  quickOpen: boolean;
  moreOpen: boolean;
}

const SLOT = "focus-ring flex min-h-14 w-full flex-col items-center justify-center gap-0.5 rounded-lg px-1 py-1.5 text-center text-xs leading-tight transition-colors duration-200";

/** Smooth to the very top; instant for people who asked for less motion. */
function scrollToTop() {
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  window.scrollTo({ top: 0, behavior: reduce ? "auto" : "smooth" });
}

function DockLink({ item }: { item: DockItem }) {
  const Icon = item.icon;
  const { pathname } = useLocation();
  // Tapping the item of the page you are already on scrolls it to the top instead of re-navigating.
  const onClick = (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (pathname !== item.path) return;
    e.preventDefault();
    scrollToTop();
  };
  return (
    <NavLink
      to={item.path}
      end={item.path === "/"}
      onClick={onClick}
      className={({ isActive }) => `${SLOT} ${isActive ? "font-semibold text-primary" : "font-medium text-muted hover:text-foreground"}`}
    >
      {({ isActive }) => (
        <>
          <Icon className="h-7 w-7 shrink-0" strokeWidth={isActive ? 2.5 : 2} aria-hidden="true" />
          <span className="break-words">{item.label}</span>
        </>
      )}
    </NavLink>
  );
}

/**
 * Mobile bottom bar (below lg): two destinations, the raised "+" for a quick expense, one more destination and "Más".
 * The whole icon-and-label block of a slot is the tap target.
 */
export function BottomDock({ role, onQuickExpense, onMore, quickOpen, moreOpen }: Props) {
  const [panel, expenses, income] = dockItems(role);
  return (
    <nav aria-label="Navegación principal" className="fixed inset-x-0 bottom-0 z-30 border-t border-border bg-surface pb-[env(safe-area-inset-bottom)] lg:hidden">
      <ul className="mx-auto grid max-w-lg grid-cols-5 items-center px-1 pt-1">
        <li>{panel && <DockLink item={panel} />}</li>
        <li>{expenses && <DockLink item={expenses} />}</li>
        <li className="flex justify-center">
          <button
            type="button"
            onClick={onQuickExpense}
            aria-label="Registrar gasto rápido"
            aria-haspopup="dialog"
            aria-expanded={quickOpen}
            className="focus-ring flex min-h-14 min-w-14 flex-col items-center justify-center gap-0.5 rounded-lg px-1 py-1.5 text-xs font-medium leading-tight text-muted"
          >
            {/* Same 28px icon box as the other slots, so the label shares their baseline; the circle rises out of it. */}
            <span className="relative h-7 w-7 shrink-0">
              <span className="absolute bottom-0 left-1/2 flex h-12 w-12 -translate-x-1/2 items-center justify-center rounded-full bg-primary text-on-primary shadow-lg shadow-foreground/25 ring-4 ring-surface">
                <Plus className="h-6 w-6" strokeWidth={2.5} aria-hidden="true" />
              </span>
            </span>
            <span aria-hidden="true">Gasto</span>
          </button>
        </li>
        <li>{income && <DockLink item={income} />}</li>
        <li>
          <button
            type="button"
            onClick={onMore}
            aria-haspopup="dialog"
            aria-expanded={moreOpen}
            className={`${SLOT} ${moreOpen ? "font-semibold text-primary" : "font-medium text-muted hover:text-foreground"}`}
          >
            <Ellipsis className="h-7 w-7 shrink-0" aria-hidden="true" />
            <span>Más</span>
          </button>
        </li>
      </ul>
    </nav>
  );
}
