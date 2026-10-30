import type { LucideIcon } from "lucide-react";
import { canOpen, modules, type ModuleTab } from "./modules";

type Role = "owner" | "household";

/** Modules the role can open, in sidebar order. */
export const modulesFor = (role: Role): ModuleTab[] => modules.filter((m) => canOpen(role, m.path));

/** Paths of the dock's four navigation slots (the quick-expense button sits between the first two and the last two). */
const DOCK_PATHS: Record<Role, readonly [string, string, string]> = {
  owner: ["/", "/gastos", "/ingresos"],
  household: ["/", "/mis-gastos", "/mis-ingresos"],
};

export interface DockItem {
  path: string;
  label: string;
  icon: LucideIcon;
}

/** Panel, expenses and income of the role (labels and icons come from modules.ts). Never lists a module the role cannot open. */
export function dockItems(role: Role): DockItem[] {
  return DOCK_PATHS[role].flatMap((path) => {
    const m = modules.find((x) => x.path === path);
    return m && canOpen(role, path) ? [{ path: m.path, label: m.label, icon: m.icon }] : [];
  });
}

/** Everything the dock does not show: what the "Más" sheet lists. */
export function moreModules(role: Role): ModuleTab[] {
  const inDock = new Set(dockItems(role).map((i) => i.path));
  return modulesFor(role).filter((m) => !inDock.has(m.path));
}
