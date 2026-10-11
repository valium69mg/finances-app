import { Info } from "lucide-react";
import type { ReactNode } from "react";

/** Short note shown when a link pointed to a record that is not in the loaded list; the page works as usual. */
export function DeepLinkNote({ children }: { children: ReactNode }) {
  return (
    <p role="status" className="mt-4 flex items-start gap-2 rounded-lg border border-border px-4 py-3 text-sm text-muted">
      <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 break-words">{children}</span>
    </p>
  );
}
