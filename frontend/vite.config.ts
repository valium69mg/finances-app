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
    "frame-ancestors 'none'",
  ].join("; ");
  return {
    name: "inject-csp",
    transformIndexHtml: (html) => html.replace("%CSP%", csp),
  };
}

export default defineConfig(({ command, mode }) => {
  const env = loadEnv(mode, ".", "VITE_");
  const apiOrigin = new URL(env.VITE_API_URL || DEFAULT_API_URL).origin;
  return {
    plugins: [react(), cspPlugin(apiOrigin, command === "build")],
    test: {
      environment: "jsdom",
      setupFiles: ["./src/test-setup.ts"],
      exclude: ["**/node_modules/**", "e2e/**", "playwright-report/**", "test-results/**"],
    },
  };
});
