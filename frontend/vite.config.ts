/// <reference types="vitest/config" />
import { defineConfig, loadEnv, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

const DEFAULT_API_URL = "http://localhost:8080";

// Injects a Content-Security-Policy whose connect-src is limited to the API origin.
// Dev mode needs inline scripts and websockets for HMR; builds stay strict.
function cspPlugin(apiOrigin: string, isBuild: boolean): Plugin {
  const script = isBuild ? "script-src 'self'" : "script-src 'self' 'unsafe-inline'";
  const connect = isBuild ? `connect-src ${apiOrigin}` : `connect-src ${apiOrigin} ws: http://localhost:*`;
  const style = isBuild ? "style-src 'self'" : "style-src 'self' 'unsafe-inline'";
  const csp = [
    "default-src 'self'",
    script,
    style,
    connect,
    "img-src 'self' data:",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    // frame-ancestors is ignored in a <meta> tag: production sends it (and the full policy)
    // as an HTTP header from nginx, see deploy/nginx/snippets/csp.conf.
  ].join("; ");
  return {
    name: "inject-csp",
    transformIndexHtml: (html) => html.replace("%CSP%", csp),
  };
}

// A relative VITE_API_URL (e.g. "/api", the same-origin production build behind nginx)
// is covered by 'self'; an absolute one is limited to its own origin.
export function apiOriginOf(apiUrl: string | undefined): string {
  const url = apiUrl || DEFAULT_API_URL;
  return url.startsWith("/") ? "'self'" : new URL(url).origin;
}

export default defineConfig(({ command, mode }) => {
  const env = loadEnv(mode, ".", "VITE_");
  const apiOrigin = apiOriginOf(env.VITE_API_URL);
  return {
    plugins: [react(), cspPlugin(apiOrigin, command === "build")],
    test: {
      environment: "jsdom",
      setupFiles: ["./src/test-setup.ts"],
      exclude: ["**/node_modules/**", "e2e/**", "playwright-report/**", "test-results/**"],
    },
  };
});
