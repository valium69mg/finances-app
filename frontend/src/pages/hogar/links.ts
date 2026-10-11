import { MONTH_PARAM, CATEGORY_PARAM } from "../../lib/monthParam";

/** The sections of the owner's "Hogar" view, in tab order. The id is what travels in `?seccion=`. */
export const HOGAR_SECTIONS = [
  { id: "resumen", label: "Resumen" },
  { id: "ingresos", label: "Ingresos" },
  { id: "gastos", label: "Gastos" },
  { id: "ahorros", label: "Ahorros" },
  { id: "gastos-futuros", label: "Gastos futuros" },
  { id: "deseos", label: "Lista de deseos" },
] as const;

export type HogarSection = (typeof HOGAR_SECTIONS)[number]["id"];

export const HOGAR_PATH = "/hogar";
export const USER_PARAM = "usuario";
export const SECTION_PARAM = "seccion";
export const DEFAULT_SECTION: HogarSection = "resumen";

/** The section named by a raw `?seccion=` value; anything unknown is the default. */
export function parseSection(raw: string | null | undefined): HogarSection {
  return HOGAR_SECTIONS.find((s) => s.id === raw)?.id ?? DEFAULT_SECTION;
}

/**
 * `/hogar?usuario=<id>&seccion=gastos&mes=2026-10&categoria=5`: a link that stays inside the Hogar view. Empty values
 * and the default section are left out.
 */
export function hogarLink(userId: string, section: HogarSection, extra: { mes?: string; categoria?: number } = {}): string {
  const params = new URLSearchParams();
  params.set(USER_PARAM, userId);
  if (section !== DEFAULT_SECTION) params.set(SECTION_PARAM, section);
  if (extra.mes) params.set(MONTH_PARAM, extra.mes);
  if (extra.categoria) params.set(CATEGORY_PARAM, String(extra.categoria));
  return `${HOGAR_PATH}?${params.toString()}`;
}
