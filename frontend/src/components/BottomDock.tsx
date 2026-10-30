import { NavLink } from "react-router-dom";
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

function DockLink({ item }: { item: DockItem }) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.path}
      end={item.path === "/"}
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
            className="focus-ring -mt-6 flex min-h-14 flex-col items-center gap-0.5 rounded-lg px-1 text-xs font-medium text-muted"
          >
            <span className="flex h-14 w-14 items-center justify-center rounded-full bg-primary text-on-primary shadow-lg shadow-foreground/25 ring-4 ring-surface">
              <Plus className="h-8 w-8" strokeWidth={2.5} aria-hidden="true" />
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
