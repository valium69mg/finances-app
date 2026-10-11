import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2, Users } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";
import { listUsers, usersKeys, type UserAccount } from "../api/users";
import { EmptyNote } from "../components/EmptyNote";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";
import { FutureBackendProvider, viewFutureBackend } from "./futureexpenses/backend";
import { FutureExpensesView } from "./futureexpenses/FutureExpensesView";
import { HOGAR_SECTIONS, SECTION_PARAM, USER_PARAM, hogarLink, parseSection, type HogarSection } from "./hogar/links";
import { Resumen } from "./hogar/Resumen";
import { ListaDeDeseos } from "./ListaDeDeseos";
import { PERSONAL_PAGES } from "./personal/config";
import { PersonalMovements } from "./personal/PersonalMovements";
import { PersonalScopeProvider } from "./personal/scope";
import { describeUsersError } from "./users/errors";
import { ErrorBanner, secondaryButton } from "./settings/ui";
import { SelectField } from "./expenses/SelectField";
import { MONTH_PARAM, parseMonthParam } from "../lib/monthParam";

/**
 * Hogar (owner only): a READ-ONLY view of the finances of a household user. The same pages she uses (summary, income,
 * expenses, savings, future expenses and wishlist) render here inside a PersonalScope, so every read carries her id
 * and nothing that writes is rendered. The selected person lives in `?usuario=`, the section in `?seccion=` and the
 * month in `?mes=`.
 */
export function Hogar() {
  const [params, setParams] = useSearchParams();
  const users = useQuery({ queryKey: usersKeys.all, queryFn: listUsers, retry: false });
  const members = useMemo(() => (users.data ?? []).filter((u) => u.role === "household" && u.active), [users.data]);

  const section = parseSection(params.get(SECTION_PARAM));
  const month = parseMonthParam(params.get(MONTH_PARAM));
  // One person is selected by itself; with several, the URL decides (an unknown or inactive id is ignored).
  const wanted = params.get(USER_PARAM);
  const selected: UserAccount | undefined = members.length === 1 ? members[0] : members.find((u) => u.id === wanted);

  function pickUser(id: string) {
    // The category filter belongs to the previous person, so it is dropped; the section and month are kept.
    const to = new URL(hogarLink(id, section, { mes: month }), window.location.origin);
    setParams(to.searchParams, { replace: true });
  }

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/hogar")}>{selected ? `Finanzas de ${selected.email}` : "Hogar"}</PageTitle>
      <p className="mt-1 text-sm text-muted">Consulta las finanzas personales de los familiares. Es solo de lectura: aquí no se puede registrar ni cambiar nada.</p>

      <div className="mt-6 space-y-6">
        {users.isPending && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando familiares…
          </p>
        )}
        {users.isError && (
          <div className="space-y-3">
            <ErrorBanner>{describeUsersError(users.error)}</ErrorBanner>
            <button type="button" onClick={() => void users.refetch()} className={secondaryButton}>
              Reintentar
            </button>
          </div>
        )}
        {users.data && members.length === 0 && (
          <div className="rounded-xl border border-border bg-surface p-4 sm:p-6">
            <EmptyNote icon={Users}>Aún no hay familiares con cuenta activa.</EmptyNote>
            <p className="text-center text-sm">
              <Link to="/configuracion" className="focus-ring rounded font-medium text-primary underline underline-offset-2">
                Agrega o activa una cuenta en Configuración &gt; Usuarios
              </Link>
            </p>
          </div>
        )}
        {members.length > 1 && (
          <div className="w-full max-w-sm">
            <SelectField label="Familiar" value={selected?.id ?? ""} onChange={(e) => e.target.value && pickUser(e.target.value)}>
              {!selected && <option value="">Elige a un familiar</option>}
              {members.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.email}
                </option>
              ))}
            </SelectField>
          </div>
        )}
        {members.length > 1 && !selected && <p className="text-sm text-muted">Elige a un familiar para ver sus finanzas.</p>}

        {selected && (
          <PersonalScopeProvider value={{ userId: selected.id }}>
            <nav aria-label="Secciones de las finanzas" className="flex flex-wrap gap-2">
              {HOGAR_SECTIONS.map((s) => {
                const current = s.id === section;
                return (
                  <Link
                    key={s.id}
                    to={hogarLink(selected.id, s.id, { mes: month })}
                    aria-current={current ? "page" : undefined}
                    className={`focus-ring inline-flex min-h-11 items-center rounded-lg border px-4 py-2 text-sm transition-colors duration-200 ${
                      current ? "border-primary bg-primary/10 font-semibold text-primary" : "border-border bg-surface text-muted hover:bg-primary/5 hover:text-foreground"
                    }`}
                  >
                    {s.label}
                  </Link>
                );
              })}
            </nav>
            <div className="rounded-xl border border-border bg-surface p-4 sm:p-6">
              <SectionBody section={section} userId={selected.id} />
            </div>
          </PersonalScopeProvider>
        )}
      </div>
    </section>
  );
}

function SectionBody({ section, userId }: { section: HogarSection; userId: string }) {
  const futureBackend = useMemo(() => viewFutureBackend(userId), [userId]);
  switch (section) {
    case "resumen":
      return <Resumen userId={userId} />;
    case "ingresos":
      return <PersonalMovements config={PERSONAL_PAGES.Ingreso} />;
    case "gastos":
      return <PersonalMovements config={PERSONAL_PAGES.Gasto} />;
    case "ahorros":
      return <PersonalMovements config={PERSONAL_PAGES.Ahorro} />;
    case "gastos-futuros":
      return (
        <FutureBackendProvider value={futureBackend}>
          <FutureExpensesView path="/hogar" title="Gastos futuros" categories={[]} />
        </FutureBackendProvider>
      );
    case "deseos":
      return <ListaDeDeseos />;
  }
}
