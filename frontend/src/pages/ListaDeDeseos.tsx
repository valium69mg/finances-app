import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Gift, Loader2 } from "lucide-react";
import { listPersonalCategories, personalKeys } from "../api/personal";
import { listWishlist, wishlistKeys, type WishItem } from "../api/wishlist";
import { EmptyNote } from "../components/EmptyNote";
import { InlineFormPanel } from "../components/InlineFormPanel";
import { PageTitle } from "../components/PageTitle";
import { moduleIcon } from "../modules";
import { formatMoney } from "./expenses/money";
import { ErrorBanner, secondaryButton } from "./settings/ui";
import { ConvertDialog } from "./wishlist/ConvertDialog";
import { describeWishError } from "./wishlist/errors";
import type { ConvertChoice } from "./wishlist/form";
import { CONVERSION_TARGET } from "./wishlist/labels";
import { WishCard } from "./wishlist/WishCard";
import { WishForm } from "./wishlist/WishForm";

const CONVERTED_NOTICE: Record<ConvertChoice, string> = {
  gasto_futuro: "se convirtió en un gasto futuro.",
  gasto: "se registró como gasto.",
  peticion: "se envió como petición al titular. Te avisaremos por correo cuando se resuelva.",
};

/**
 * Lista de deseos (household only): what she would like to buy, with an estimated price, a category of hers, a store
 * link, a priority and a note. An active item converts into her future expense, her Gasto or a request to the owner;
 * it then moves to the converted section and comes back when the request is rejected or cancelled or its target is
 * deleted (the server derives the status).
 */
export function ListaDeDeseos() {
  const [editing, setEditing] = useState<WishItem | null>(null);
  // The create form stays collapsed until asked for; editing a row opens it by itself.
  const [formOpen, setFormOpen] = useState(false);
  const [converting, setConverting] = useState<WishItem | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const list = useQuery({ queryKey: wishlistKeys.all, queryFn: listWishlist, retry: false });
  const categories = useQuery({ queryKey: personalKeys.categories("Gasto"), queryFn: () => listPersonalCategories("Gasto"), retry: false });

  const active = list.data?.filter((i) => i.status === "activa") ?? [];
  const converted = list.data?.filter((i) => i.status === "convertida") ?? [];

  return (
    <section aria-labelledby="page-title">
      <PageTitle icon={moduleIcon("/lista-de-deseos")}>Lista de deseos</PageTitle>
      <p className="mt-1 text-sm text-muted">
        Anota lo que te gustaría comprar, con su precio estimado y qué tan pronto lo quieres. Cuando llegue el momento, conviértelo en un gasto futuro, regístralo
        como comprado o pídelo al titular.
      </p>

      <div className="mt-6 space-y-6 rounded-xl border border-border bg-surface p-4 sm:p-6">
        {(list.isPending || categories.isPending) && (
          <p role="status" className="flex items-center gap-2 text-sm text-muted">
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Cargando tu lista de deseos…
          </p>
        )}
        {(list.isError || categories.isError) && (
          <div className="space-y-3">
            <ErrorBanner>{describeWishError(list.error ?? categories.error)}</ErrorBanner>
            <button
              type="button"
              onClick={() => {
                void list.refetch();
                void categories.refetch();
              }}
              className={secondaryButton}
            >
              Reintentar
            </button>
          </div>
        )}

        {list.data && categories.data && (
          <>
            <InlineFormPanel id="wish-form-panel" label="+ Nuevo deseo" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={editing !== null}>
              <WishForm
                key={editing?.id ?? "new"}
                categories={categories.data}
                editing={editing}
                onSaved={(item, wasEdit) => {
                  setEditing(null);
                  setFormOpen(false);
                  setNotice(wasEdit ? `Cambios de ${item.name} guardados.` : `${item.name} se agregó a tu lista.`);
                }}
                onCancelEdit={() => {
                  setEditing(null);
                  setFormOpen(false);
                }}
              />
            </InlineFormPanel>

            {notice && (
              <p role="status" className="flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                <span className="min-w-0 break-words">{notice}</span>
              </p>
            )}

            <div className="space-y-3">
              <h2 className="text-base font-semibold tracking-tight">Por comprar</h2>
              {active.length === 0 ? (
                <EmptyNote icon={Gift}>Aún no tienes deseos pendientes. Agrega el primero con el botón de arriba.</EmptyNote>
              ) : (
                <>
                  <ul aria-label="Deseos activos" className="divide-y divide-border rounded-xl border border-border">
                    {active.map((item) => (
                      <WishCard
                        key={item.id}
                        item={item}
                        onConvert={(i) => {
                          setNotice(null);
                          setConverting(i);
                        }}
                        onEdit={(i) => {
                          setNotice(null);
                          setEditing(i);
                        }}
                        onDeleted={(i) => {
                          setEditing((cur) => (cur?.id === i.id ? null : cur));
                          setNotice(`${i.name} se eliminó de tu lista.`);
                        }}
                      />
                    ))}
                  </ul>
                  <p className="text-sm text-muted">
                    Total estimado: <strong className="tabular-nums text-foreground">{formatMoney(sum(active))} MXN</strong>
                  </p>
                </>
              )}
            </div>

            {converted.length > 0 && (
              <div className="space-y-3">
                <h2 className="text-base font-semibold tracking-tight">Convertidos</h2>
                <ul aria-label="Deseos convertidos" className="divide-y divide-border rounded-xl border border-border">
                  {converted.map((item) => (
                    <WishCard
                      key={item.id}
                      item={item}
                      onConvert={setConverting}
                      onEdit={(i) => {
                        setNotice(null);
                        setEditing(i);
                      }}
                      onDeleted={(i) => {
                        setEditing((cur) => (cur?.id === i.id ? null : cur));
                        setNotice(`${i.name} se eliminó de tu lista. Lo que se creó con él no cambió.`);
                      }}
                    />
                  ))}
                </ul>
              </div>
            )}
          </>
        )}
      </div>

      {converting && (
        <ConvertDialog
          key={converting.id}
          item={converting}
          onClose={() => setConverting(null)}
          onConverted={(item, choice) => {
            setConverting(null);
            setEditing((cur) => (cur?.id === item.id ? null : cur));
            const where = item.conversion ? CONVERSION_TARGET[item.conversion.type].link.replace("Ver en ", "") : "";
            setNotice(`${item.name} ${CONVERTED_NOTICE[choice]}${choice === "peticion" ? "" : ` Lo encuentras en ${where}.`}`);
          }}
        />
      )}
    </section>
  );
}

/** Sum of the estimated prices in cents, as a decimal string (no floats). */
function sum(items: WishItem[]): string {
  const cents = items.reduce((total, i) => {
    const m = /^(\d+)(?:\.(\d+))?$/.exec(i.price);
    return m ? total + BigInt(m[1] + (m[2] ?? "").padEnd(2, "0").slice(0, 2)) : total;
  }, 0n);
  const s = cents.toString().padStart(3, "0");
  return `${s.slice(0, -2)}.${s.slice(-2)}`;
}
