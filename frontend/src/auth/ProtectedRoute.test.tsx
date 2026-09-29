import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { AuthProvider } from "./AuthContext";
import { ProtectedRoute } from "./ProtectedRoute";

function renderGuard() {
  render(
    <AuthProvider>
      <MemoryRouter initialEntries={["/secret"]}>
        <Routes>
          <Route path="/login" element={<p>login page</p>} />
          <Route element={<ProtectedRoute />}>
            <Route path="/secret" element={<p>secret page</p>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </AuthProvider>,
  );
}

describe("ProtectedRoute", () => {
  beforeEach(() => localStorage.clear());

  it("redirects to login without a session", () => {
    renderGuard();
    expect(screen.getByText("login page")).toBeInTheDocument();
  });

  it("renders the protected content with a session", () => {
    localStorage.setItem("refresh_token", "r");
    localStorage.setItem("access_token", "a");
    renderGuard();
    expect(screen.getByText("secret page")).toBeInTheDocument();
  });
});
