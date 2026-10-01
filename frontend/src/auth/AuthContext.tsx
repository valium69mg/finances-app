import { useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { SESSION_EVENT, tokenStore, type Role } from "../api/tokens";
import { logout as apiLogout } from "../api/auth";

interface AuthState {
  isAuthenticated: boolean;
  /**
   * Role of the session. A session stored before roles existed has none until the next refresh brings it;
   * it is treated as the owner meanwhile (the backend is the authority and answers 403 otherwise).
   */
  role: Role;
  markLoggedIn: () => void;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setAuthenticated] = useState(() => tokenStore.getRefresh() !== null);
  const [role, setRole] = useState<Role>(() => tokenStore.getRole() ?? "owner");
  const queryClient = useQueryClient();

  useEffect(() => {
    // Cached answers belong to the account that fetched them: never show them to the next one to sign in.
    const onExpired = () => {
      setAuthenticated(false);
      queryClient.clear();
    };
    // Login, refresh and /auth/me write the role to the store: follow it.
    const onSession = () => setRole(tokenStore.getRole() ?? "owner");
    window.addEventListener("auth:expired", onExpired);
    window.addEventListener(SESSION_EVENT, onSession);
    return () => {
      window.removeEventListener("auth:expired", onExpired);
      window.removeEventListener(SESSION_EVENT, onSession);
    };
  }, [queryClient]);

  const markLoggedIn = useCallback(() => setAuthenticated(true), []);
  const logout = useCallback(async () => {
    await apiLogout();
    setAuthenticated(false);
    queryClient.clear();
  }, [queryClient]);

  const value = useMemo(() => ({ isAuthenticated, role, markLoggedIn, logout }), [isAuthenticated, role, markLoggedIn, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
