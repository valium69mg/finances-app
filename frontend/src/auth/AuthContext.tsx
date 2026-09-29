import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { tokenStore } from "../api/tokens";
import { logout as apiLogout } from "../api/auth";

interface AuthState {
  isAuthenticated: boolean;
  markLoggedIn: () => void;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setAuthenticated] = useState(() => tokenStore.getRefresh() !== null);

  useEffect(() => {
    const onExpired = () => setAuthenticated(false);
    window.addEventListener("auth:expired", onExpired);
    return () => window.removeEventListener("auth:expired", onExpired);
  }, []);

  const markLoggedIn = useCallback(() => setAuthenticated(true), []);
  const logout = useCallback(async () => {
    await apiLogout();
    setAuthenticated(false);
  }, []);

  const value = useMemo(() => ({ isAuthenticated, markLoggedIn, logout }), [isAuthenticated, markLoggedIn, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
