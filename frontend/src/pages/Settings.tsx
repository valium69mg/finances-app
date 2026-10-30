import { useRef, type KeyboardEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { getSettings, settingsKeys, type AllSettings } from "../api/settings";
import { BracketsSection } from "./settings/BracketsSection";
import { CategoriesSection } from "./settings/CategoriesSection";
import { ClientsSection } from "./settings/ClientsSection";
import { GeneralSection } from "./settings/GeneralSection";
import { InstrumentsSection } from "./settings/InstrumentsSection";
import { PauseSection } from "./settings/PauseSection";
import { UsersSection } from "./users/UsersSection";
import { SelectField } from "./expenses/SelectField";
import { ErrorBanner, describeSaveError, secondaryButton } from "./settings/ui";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";

/** The settings sections; a `standalone` one loads its own data, so it neither waits for nor needs the settings query. */
const SECTIONS = [
  { id: "general", label: "General", render: (s: AllSettings) => <GeneralSection data={s.general} /> },
  { id: "categorias", label: "Categorías y presupuestos", render: (s: AllSettings) => <CategoriesSection data={s.categories} general={s.general} /> },
  { id: "clientes", label: "Clientes", render: (s: AllSettings) => <ClientsSection data={s.clients} /> },
  { id: "instrumentos", label: "Instrumentos", render: (s: AllSettings) => <InstrumentsSection data={s.instruments} /> },
  { id: "resico", label: "Rangos de RESICO", render: (s: AllSettings) => <BracketsSection data={s.brackets} /> },
  { id: "pausa", label: "Pausa de inversiones", render: (s: AllSettings) => <PauseSection data={s.investment_pause} /> },
  { id: "usuarios", label: "Usuarios", standalone: true, render: () => <UsersSection /> },
] as const;

type SectionId = (typeof SECTIONS)[number]["id"];

/** Section for a `?seccion=` value; anything unknown (or missing) opens General. */
export function sectionFromParam(value: string | null): SectionId {
  return SECTIONS.find((s) => s.id === value)?.id ?? "general";
}

export function Settings() {
  const [params, setParams] = useSearchParams();
  const active = sectionFromParam(params.get("seccion"));
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  const select = (id: SectionId) => setParams({ seccion: id }, { replace: true });

  function onTabKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = SECTIONS.length - 1;
    const prev = e.key === "ArrowUp" || e.key === "ArrowLeft";
    const next = e.key === "ArrowDown" || e.key === "ArrowRight";
    const target = prev ? (index === 0 ? last : index - 1) : next ? (index === last ? 0 : index + 1) : e.key === "Home" ? 0 : e.key === "End" ? last : null;
    if (target === null) return;
    e.preventDefault();
    select(SECTIONS[target].id);
    tabRefs.current[SECTIONS[target].id]?.focus();
  }

  const current = SECTIONS.find((s) => s.id === active) ?? SECTIONS[0];
  const standalone = "standalone" in current && current.standalone;

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/configuracion")}>Configuración</PageTitle>
      <p className="mt-1 text-sm text-muted">Ajusta los parámetros que usan todos los módulos.</p>

      <div className="mt-6 lg:grid lg:grid-cols-[15rem_minmax(0,1fr)] lg:gap-8">
        <div className="lg:hidden">
          <SelectField label="Sección" value={active} onChange={(e) => select(sectionFromParam(e.target.value))}>
            {SECTIONS.map((s) => (
              <option key={s.id} value={s.id}>
                {s.label}
              </option>
            ))}
          </SelectField>
        </div>

        <div role="tablist" aria-orientation="vertical" aria-label="Secciones de configuración" className="sticky top-4 hidden self-start lg:flex lg:flex-col lg:gap-1">
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
                onClick={() => select(s.id)}
                onKeyDown={(e) => onTabKeyDown(e, i)}
                className={`focus-ring min-h-11 rounded-lg border-l-4 px-4 py-2 text-left text-sm transition-colors duration-200 ${
                  selected ? "border-primary bg-primary/10 font-semibold text-primary" : "border-transparent text-muted hover:bg-primary/5 hover:text-foreground"
                }`}
              >
                {s.label}
              </button>
            );
          })}
        </div>

        <div role="tabpanel" id={`panel-${current.id}`} aria-labelledby={`tab-${current.id}`} tabIndex={-1} className="mt-6 min-w-0 lg:col-start-2 lg:row-start-1 lg:mt-0">
          {standalone && <UsersSection />}
          {!standalone && settings.isPending && (
            <p role="status" className="flex items-center gap-2 text-sm text-muted">
              <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              Cargando configuración…
            </p>
          )}
          {!standalone && settings.isError && (
            <div className="space-y-3">
              <ErrorBanner>{describeSaveError(settings.error)}</ErrorBanner>
              <button type="button" onClick={() => void settings.refetch()} className={secondaryButton}>
                Reintentar
              </button>
            </div>
          )}
          {!standalone && settings.data && current.render(settings.data)}
        </div>
      </div>
    </section>
  );
}
