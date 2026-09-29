import { Construction } from "lucide-react";
import { Link } from "react-router-dom";
import type { ModuleTab } from "../modules";

export function Placeholder({ module }: { module: ModuleTab }) {
  const Icon = module.icon;
  return (
    <section aria-labelledby="page-title">
      <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
        {module.label}
      </h1>
      <div className="mt-6 flex flex-col items-center rounded-xl border border-dashed border-border bg-surface px-6 py-12 text-center">
        <span className="flex h-14 w-14 items-center justify-center rounded-full bg-accent/10 text-accent" aria-hidden="true">
          <Icon className="h-7 w-7" />
        </span>
        <h2 className="mt-4 flex items-center gap-2 text-lg font-semibold">
          <Construction className="h-5 w-5 text-muted" aria-hidden="true" />
          Este módulo estará disponible próximamente
        </h2>
        <p className="mt-2 max-w-md text-muted">{module.description}</p>
        {module.path !== "/" && (
          <Link
            to="/"
            className="focus-ring mt-6 inline-flex min-h-11 items-center rounded-lg border border-border px-4 text-sm font-medium transition-colors duration-200 hover:bg-primary/10"
          >
            Volver al panel
          </Link>
        )}
      </div>
    </section>
  );
}
