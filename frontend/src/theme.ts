import { useSyncExternalStore } from "react";

/** "auto" follows prefers-color-scheme (no data-theme attribute); the others force a scheme on this device. */
export type ThemePreference = "auto" | "light" | "dark";

export const THEME_KEY = "theme";

export const THEME_OPTIONS: { value: ThemePreference; label: string }[] = [
  { value: "auto", label: "Automático" },
  { value: "light", label: "Claro" },
  { value: "dark", label: "Oscuro" },
];

/** Same values as the two <meta name="theme-color"> tags in index.html (background token of each scheme). */
const FORCED_COLOR = { light: "#F8FAFC", dark: "#0B1120" } as const;

const isPreference = (v: unknown): v is ThemePreference => v === "auto" || v === "light" || v === "dark";

/** The stored preference; anything missing, unknown or unreadable (private mode) means "auto". */
export function readTheme(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(THEME_KEY);
    return isPreference(stored) ? stored : "auto";
  } catch {
    return "auto";
  }
}

/** Sets or clears data-theme on <html> and aligns the browser chrome color (theme-color) with a forced scheme. */
export function applyTheme(pref: ThemePreference): void {
  const root = document.documentElement;
  if (pref === "auto") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", pref);

  document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]').forEach((meta) => {
    // The original content is kept so "auto" can restore it.
    if (meta.dataset.autoColor === undefined) meta.dataset.autoColor = meta.content;
    meta.content = pref === "auto" ? meta.dataset.autoColor : FORCED_COLOR[pref];
  });
}

const listeners = new Set<() => void>();
let current: ThemePreference | null = null;

const snapshot = (): ThemePreference => (current ??= readTheme());

/** Persists the preference on this device, applies it and tells every mounted selector. */
export function setTheme(pref: ThemePreference): void {
  try {
    if (pref === "auto") window.localStorage.removeItem(THEME_KEY);
    else window.localStorage.setItem(THEME_KEY, pref);
  } catch {
    // Storage unavailable: the choice still applies until the page is closed.
  }
  current = pref;
  applyTheme(pref);
  listeners.forEach((l) => l());
}

/** Applies the stored preference; called once at startup (public/theme-init.js already did it before first paint). */
export function initTheme(): void {
  current = readTheme();
  applyTheme(current);
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useTheme(): [ThemePreference, (pref: ThemePreference) => void] {
  return [useSyncExternalStore(subscribe, snapshot, () => "auto"), setTheme];
}

/** Test hook: forget the cached preference so the next read goes to storage again. */
export function resetThemeCache(): void {
  current = null;
}
