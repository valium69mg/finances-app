import { createContext, useContext } from "react";

/**
 * Whose personal finances a page shows. Her own pages live outside any provider (`userId` null: her session, she can
 * write). The owner's "Hogar" view wraps the same pages in a provider with the household user's id: every read then
 * carries `user_id` and the page is READ-ONLY, meaning the write controls are not rendered at all.
 */
export interface PersonalScope {
  /** The household user the owner is looking at; null in her own pages. */
  userId: string | null;
}

const PersonalScopeContext = createContext<PersonalScope>({ userId: null });

export const PersonalScopeProvider = PersonalScopeContext.Provider;

export function usePersonalScope(): PersonalScope & { readOnly: boolean } {
  const scope = useContext(PersonalScopeContext);
  return { ...scope, readOnly: scope.userId !== null };
}
