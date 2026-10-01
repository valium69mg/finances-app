import { Car, Gift, GraduationCap, HeartPulse, House, PawPrint, Plane, Shirt, ShoppingCart, Smartphone, Sparkles, Utensils, Wrench, Zap, type LucideIcon } from "lucide-react";

/** Lowercase and accent-free, so "Educación" and "educacion" match the same keyword. */
export function normalizeCategory(name: string): string {
  return name
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase();
}

// First match wins. Every pattern is anchored to the start of a word so "super" matches "Supermercado"
// but "auto" does not match "Restaurante autóctono"; prefixes cover plurals and derived forms.
const RULES: ReadonlyArray<readonly [RegExp, LucideIcon]> = [
  [/\b(comida|restaurante|restaurant)/, Utensils],
  [/\b(super|despensa|mandado)/, ShoppingCart],
  [/\b(transporte|auto\b|autos\b|gasolina)/, Car],
  [/\b(casa|renta|hogar|vivienda)/, House],
  [/\b(servicio|luz\b)/, Zap],
  [/\b(salud|medic)/, HeartPulse],
  [/\b(ropa)/, Shirt],
  [/\b(viaje)/, Plane],
  [/\b(celular|suscrip|telefono)/, Smartphone],
  [/\b(regalo)/, Gift],
  [/\b(educacion|curso)/, GraduationCap],
  [/\b(mantenimiento)/, Wrench],
  [/\b(mascota)/, PawPrint],
];

/** Icon for a spending category, by keyword on its name; anything unknown gets the generic sparkles. */
export function categoryIcon(name: string): LucideIcon {
  const n = normalizeCategory(name);
  for (const [pattern, icon] of RULES) if (pattern.test(n)) return icon;
  return Sparkles;
}
