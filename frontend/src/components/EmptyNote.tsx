import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

/** Empty state: a small muted icon above the message. */
export function EmptyNote({ icon: Icon, children }: { icon: LucideIcon; children: ReactNode }) {
  return (
    <div className="mt-2 flex flex-col items-center gap-2 py-3 text-center text-sm text-muted">
      <Icon className="h-6 w-6" aria-hidden="true" />
      <p className="min-w-0 break-words">{children}</p>
    </div>
  );
}
