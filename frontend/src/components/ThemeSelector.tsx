import { useId } from "react";
import { Monitor, Moon, Sun, type LucideIcon } from "lucide-react";
import { THEME_OPTIONS, useTheme, type ThemePreference } from "../theme";

const ICONS: Record<ThemePreference, LucideIcon> = { auto: Monitor, light: Sun, dark: Moon };

/**
 * Segmented control Automático / Claro / Oscuro; native radios give the arrow-key behavior. Stored per device.
 * `compact` is the icon-only version of the desktop navbar (the label stays as the accessible name and tooltip).
 */
export function ThemeSelector({ compact = false }: { compact?: boolean }) {
  const [theme, setTheme] = useTheme();
  const name = useId();
  if (compact) {
    return (
      <div role="radiogroup" aria-label="Tema" className="inline-flex gap-0.5 rounded-lg border border-border bg-background p-0.5">
        {THEME_OPTIONS.map(({ value, label }) => {
          const Icon = ICONS[value];
          return (
            <label
              key={value}
              title={label}
              className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-md text-muted transition-colors duration-200 hover:text-foreground has-[:checked]:bg-primary/15 has-[:checked]:text-primary has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-1 has-[:focus-visible]:outline-ring"
            >
              <input type="radio" name={name} value={value} checked={theme === value} onChange={() => setTheme(value)} className="sr-only" />
              <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span className="sr-only">{label}</span>
            </label>
          );
        })}
      </div>
    );
  }
  return (
    <div>
      <p id={`${name}-label`} className="mb-1.5 text-xs font-medium text-muted">
        Tema
      </p>
      <div role="radiogroup" aria-labelledby={`${name}-label`} className="grid grid-cols-3 gap-1 rounded-lg border border-border bg-background p-1">
        {THEME_OPTIONS.map(({ value, label }) => {
          const Icon = ICONS[value];
          return (
            <label
              key={value}
              className="flex min-h-14 cursor-pointer flex-col items-center justify-center gap-0.5 rounded-md px-1 text-xs font-medium text-muted transition-colors duration-200 hover:text-foreground has-[:checked]:bg-primary/15 has-[:checked]:text-primary has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-1 has-[:focus-visible]:outline-ring"
            >
              <input type="radio" name={name} value={value} checked={theme === value} onChange={() => setTheme(value)} className="sr-only" />
              <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
              {label}
            </label>
          );
        })}
      </div>
    </div>
  );
}
