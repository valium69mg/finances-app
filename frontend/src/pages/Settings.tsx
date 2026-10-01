import { useRef, useState, type KeyboardEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { getSettings, settingsKeys, type AllSettings } from "../api/settings";
import { BracketsSection } from "./settings/BracketsSection";
import { CategoriesSection } from "./settings/CategoriesSection";
import { ClientsSection } from "./settings/ClientsSection";
import { GeneralSection } from "./settings/GeneralSection";
import { InstrumentsSection } from "./settings/InstrumentsSection";
import { PauseSection } from "./settings/PauseSection";
import { ErrorBanner, describeSaveError, secondaryButton } from "./settings/ui";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";

const SECTIONS = [
  { id: "general", label: "General", render: (s: AllSettings) => <GeneralSection data={s.general} /> },
  { id: "categorias", label: "Categorías y presupuestos", render: (s: AllSettings) => <CategoriesSection data={s.categories} general={s.general} /> },
  { id: "clientes", label: "Clientes", render: (s: AllSettings) => <ClientsSection data={s.clients} /> },
  { id: "instrumentos", label: "Instrumentos", render: (s: AllSettings) => <InstrumentsSection data={s.instruments} /> },
  { id: "resico", label: "Rangos de RESICO", render: (s: AllSettings) => <BracketsSection data={s.brackets} /> },
  { id: "pausa", label: "Pausa de inversiones", render: (s: AllSettings) => <PauseSection data={s.investment_pause} /> },
] as const;

type SectionId = (typeof SECTIONS)[number]["id"];

export function Settings() {
  const [active, setActive] = useState<SectionId>("general");
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  function onTabKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = SECTIONS.length - 1;
    const next =
      e.key === "ArrowRight" ? (index === last ? 0 : index + 1) : e.key === "ArrowLeft" ? (index === 0 ? last : index - 1) : e.key === "Home" ? 0 : e.key === "End" ? last : null;
    if (next === null) return;
    e.preventDefault();
    setActive(SECTIONS[next].id);
    tabRefs.current[SECTIONS[next].id]?.focus();
  }

  const current = SECTIONS.find((s) => s.id === active) ?? SECTIONS[0];

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/configuracion")}>Configuración</PageTitle>
      <p className="mt-1 text-sm text-muted">Ajusta los parámetros que usan todos los módulos.</p>

      <div role="tablist" aria-label="Secciones de configuración" className="mt-6 flex flex-wrap gap-2">
        {SECTIONS.map((s, i) => {
          const selected = s.id === active;
          return (
            <button
              key={s.id}
              ref={(el) => {
                tabRefs.current[s.id] = el;
              }}
              type="button"
              role="tab"
              id={`tab-${s.id}`}
              aria-selected={selected}
              aria-controls={`panel-${s.id}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => setActive(s.id)}
              onKeyDown={(e) => onTabKeyDown(e, i)}
              className={`focus-ring min-h-11 rounded-lg border px-4 py-2 text-sm transition-colors duration-200 ${
                selected ? "border-primary bg-primary/10 font-semibold text-primary" : "border-border bg-surface text-muted hover:bg-primary/5 hover:text-foreground"
              }`}
            >
              {s.label}
            </button>
          );
        })}
      </div>

      <div role="tabpanel" id={`panel-${current.id}`} aria-labelledby={`tab-${current.id}`} tabIndex={-1} className="mt-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {settings.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando configuración…
          </p>
        )}
        {settings.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeSaveError(settings.error)}</ErrorBanner>
            <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {settings.data && current.render(settings.data)}
      </div>
    </section>
  );
}
