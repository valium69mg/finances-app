import { ChevronRight, ExternalLink } from "lucide-react";
import { Link } from "react-router-dom";
import type { WishItem } from "../../api/wishlist";
import { formatMoney } from "../expenses/money";
import { StateBadge } from "../expenserequests/labels";
import { rowButton } from "../settings/ui";
import { hogarLink } from "../hogar/links";
import { usePersonalScope } from "../personal/scope";
import { CONVERSION_TARGET, CONVERSION_VIEW, PriorityBadge, returnedNote, returnedViewNote } from "./labels";

/** True for a link the browser may open from the page: http or https only (the backend already enforces it). */
const safeUrl = (u: string | null): u is string => u !== null && /^https?:\/\//i.test(u);

/** What a converted item became (text, link label and the in-app path), in the wording of whoever is looking. */
export function useConversionTarget(item: WishItem) {
  const { userId, readOnly } = usePersonalScope();
  if (!item.conversion) return null;
  const view = CONVERSION_VIEW[item.conversion.type];
  return readOnly ? { text: view.text, link: view.link, path: view.section && userId ? hogarLink(userId, view.section) : "" } : CONVERSION_TARGET[item.conversion.type];
}

/** One-line status of a card: what a converted item became, or why an active one came back. Plain text (no links). */
function useStatusNote(item: WishItem): string | null {
  const target = useConversionTarget(item);
  const { readOnly } = usePersonalScope();
  const returned = readOnly ? returnedViewNote(item) : returnedNote(item);
  if (item.status === "convertida" && target) return target.text;
  return returned;
}

/** The store link, the note and (when converted) where it went, with links. Shown in the detail dialog and in the read-only card. */
export function WishDetails({ item }: { item: WishItem }) {
  const target = useConversionTarget(item);
  const { readOnly } = usePersonalScope();
  const returned = readOnly ? returnedViewNote(item) : returnedNote(item);
  const converted = item.status === "convertida";
  return (
    <div className="space-y-3">
      {(safeUrl(item.store_url) || item.note) && (
        <div className="space-y-1 text-sm">
          {safeUrl(item.store_url) && (
            <a href={item.store_url} target="_blank" rel="noopener noreferrer" className="focus-ring inline-flex min-h-6 items-center gap-1.5 rounded text-primary underline underline-offset-2">
              <ExternalLink className="h-4 w-4 shrink-0" aria-hidden="true" />
              Ver en la tienda
              <span className="sr-only"> (se abre en una pestaña nueva)</span>
            </a>
          )}
          {item.note && <p className="break-words text-muted">{item.note}</p>}
        </div>
      )}

      {converted && target && item.conversion && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
          <span>{target.text}</span>
          {item.conversion.type === "peticion" && item.conversion.request_state && <StateBadge status={item.conversion.request_state} />}
          {target.path && (
            <Link to={target.path} className="focus-ring rounded text-primary underline underline-offset-2">
              {target.link}
            </Link>
          )}
        </div>
      )}
      {returned && <p className="text-sm text-muted">{returned}</p>}
    </div>
  );
}

function CardBody({ item, trailing }: { item: WishItem; trailing?: boolean }) {
  // The plain read-only card carries the full details under it, so only the button card repeats the status here.
  const status = useStatusNote(item);
  const showStatus = trailing === true;
  return (
    <>
      <span className="min-w-0 flex-1">
        <span className="block break-words font-medium">{item.name}</span>
        <span className="block text-sm tabular-nums text-muted">
          {formatMoney(item.price)} MXN · {item.category}
        </span>
        {showStatus && status && <span className="mt-1 block text-sm text-muted">{status}</span>}
      </span>
      <PriorityBadge priority={item.priority} />
      {item.status === "convertida" && item.conversion?.type === "peticion" && item.conversion.request_state && <StateBadge status={item.conversion.request_state} />}
      {trailing && <ChevronRight className="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />}
    </>
  );
}

interface Props {
  item: WishItem;
  /** Opens the detail dialog with the item's actions (hers only; read-only renders the whole card with its links). */
  onOpen: (item: WishItem) => void;
}

/**
 * One wishlist item as a summary card: name, price and category, priority and status. Hers is one button that opens
 * the detail dialog (convert, edit, delete); in the owner's read-only view it is a plain card with everything on it.
 */
export function WishCard({ item, onOpen }: Props) {
  const { readOnly } = usePersonalScope();
  if (readOnly) {
    return (
      <li data-testid="wish-card" data-status={item.status} className="space-y-3 rounded-xl border border-border bg-surface px-4 py-3">
        <div className="flex flex-wrap items-start gap-3">
          <CardBody item={item} />
        </div>
        <WishDetails item={item} />
      </li>
    );
  }
  return (
    <li data-testid="wish-card" data-status={item.status}>
      <button type="button" onClick={() => onOpen(item)} aria-label={`Ver ${item.name}`} className={rowButton}>
        <CardBody item={item} trailing />
      </button>
    </li>
  );
}
