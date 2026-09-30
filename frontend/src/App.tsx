import { Navigate, Route, Routes } from "react-router-dom";
import { ProtectedRoute } from "./auth/ProtectedRoute";
import { Layout } from "./components/Layout";
import { modules } from "./modules";
import { Expenses } from "./pages/Expenses";
import { Login } from "./pages/Login";
import { Placeholder } from "./pages/Placeholder";
import { Settings } from "./pages/Settings";
import { VerifyEmail } from "./pages/VerifyEmail";

function pageFor(path: string) {
  if (path === "/configuracion") return <Settings />;
  if (path === "/gastos") return <Expenses />;
  return null;
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/verify" element={<VerifyEmail />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          {modules.map((m) => (
            <Route key={m.path} path={m.path} element={pageFor(m.path) ?? <Placeholder module={m} />} />
          ))}
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
