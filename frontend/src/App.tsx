import { Navigate, Route, Routes } from "react-router-dom";
import { ProtectedRoute } from "./auth/ProtectedRoute";
import { Layout } from "./components/Layout";
import { modules } from "./modules";
import { Login } from "./pages/Login";
import { Placeholder } from "./pages/Placeholder";
import { VerifyEmail } from "./pages/VerifyEmail";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/verify" element={<VerifyEmail />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          {modules.map((m) => (
            <Route key={m.path} path={m.path} element={<Placeholder title={m.label} />} />
          ))}
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
