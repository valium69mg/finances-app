import { ArrowDown, ArrowUp, Minus, type LucideIcon } from "lucide-react";
import type { WishConversion, WishItem, WishPriority } from "../../api/wishlist";
import { TONES } from "../../lib/tones";
import type { HogarSection } from "../hogar/links";

interface PriorityMeta {
  label: string;
  icon: LucideIcon;
  className: string;
}

/** Name of each priority in a select. */
export const PRIORITY_NAME: Record<WishPriority, string> = { alta: "Alta", media: "Media", baja: "Baja" };

/** Icon, text and color of each priority: the priority is never told by color alone. */
export const PRIORITY_META: Record<WishPriority, PriorityMeta> = {
  alta: { label: "Prioridad alta", icon: ArrowUp, className: TONES.expense.chip },
  media: { label: "Prioridad media", icon: Minus, className: TONES.primary.chip },
  baja: { label: "Prioridad baja", icon: ArrowDown, className: "bg-border/50 text-muted" },
};

export function PriorityBadge({ priority }: { priority: WishPriority }) {
  const { label, icon: Icon, className } = PRIORITY_META[priority];
  return (
    <span data-testid="wish-priority" className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${className}`}>
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      {label}
    </span>
  );
}

/** Where each kind of conversion lives, with the label of the link. */
export const CONVERSION_TARGET: Record<WishConversion["type"], { text: string; path: string; link: string }> = {
  gasto_futuro: { text: "Se convirtió en un gasto futuro.", path: "/mis-gastos-futuros", link: "Ver en Mis gastos futuros" },
  gasto: { text: "Ya lo compraste: se registró como gasto.", path: "/mis-gastos", link: "Ver en Mis gastos" },
  peticion: { text: "Se pidió al titular.", path: "/peticiones", link: "Ver en Peticiones" },
};

/** The request of an active item that came back: rejected or cancelled, so it can be converted again. */
export function returnedNote(item: WishItem): string | null {
  const c = item.conversion;
  if (item.status !== "activa" || !c || c.type !== "peticion") return null;
  if (c.request_state === "rechazada") return "El titular rechazó tu petición: puedes convertirlo de nuevo.";
  if (c.request_state === "cancelada") return "Cancelaste tu petición: puedes convertirlo de nuevo.";
  return null;
}

/** Read-only wording of the owner's view: what the item became, in the third person, and the Hogar section behind it. */
export const CONVERSION_VIEW: Record<WishConversion["type"], { text: string; section: HogarSection | null; link: string }> = {
  gasto_futuro: { text: "Se convirtió en un gasto futuro.", section: "gastos-futuros", link: "Ver en Gastos futuros" },
  gasto: { text: "Se registró como gasto.", section: "gastos", link: "Ver en Gastos" },
  // The request lives in the family Peticiones, which this view does not reproduce.
  peticion: { text: "Se pidió al titular.", section: null, link: "" },
};

/** Read-only wording of {@link returnedNote}. */
export function returnedViewNote(item: WishItem): string | null {
  const c = item.conversion;
  if (item.status !== "activa" || !c || c.type !== "peticion") return null;
  if (c.request_state === "rechazada") return "El titular rechazó la petición: el deseo volvió a pendientes.";
  if (c.request_state === "cancelada") return "La petición se canceló: el deseo volvió a pendientes.";
  return null;
}
