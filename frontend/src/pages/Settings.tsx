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
import { SectionNav } from "../components/SectionNav";
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
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });

  const select = (id: SectionId) => setParams({ seccion: id }, { replace: true });

  const current = SECTIONS.find((s) => s.id === active) ?? SECTIONS[0];
  const standalone = "standalone" in current && current.standalone;

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/configuracion")}>Configuración</PageTitle>
      <p className="mt-1 text-sm text-muted">Ajusta los parámetros que usan todos los módulos.</p>

      <div className="mt-6">
        <SectionNav items={SECTIONS} active={active} onSelect={select} label="Secciones de configuración">
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
        </SectionNav>
      </div>
    </section>
  );
}
