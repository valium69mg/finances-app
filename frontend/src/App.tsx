import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth/AuthContext";
import { ProtectedRoute } from "./auth/ProtectedRoute";
import { Layout } from "./components/Layout";
import { canOpen, modules } from "./modules";
import { Bills } from "./pages/Bills";
import { Dashboard } from "./pages/Dashboard";
import { ExpenseRequests } from "./pages/ExpenseRequests";
import { Expenses } from "./pages/Expenses";
import { FutureExpenses } from "./pages/FutureExpenses";
import { FiledRecords } from "./pages/FiledRecords";
import { Income } from "./pages/Income";
import { Invoices } from "./pages/Invoices";
import { Login } from "./pages/Login";
import { MonthClose } from "./pages/MonthClose";
import { Placeholder } from "./pages/Placeholder";
import { Savings } from "./pages/Savings";
import { Settings } from "./pages/Settings";
import { System } from "./pages/System";
import { TaxFiling } from "./pages/TaxFiling";
import { VerifyEmail } from "./pages/VerifyEmail";

function pageFor(path: string) {
  if (path === "/") return <Dashboard />;
  if (path === "/peticiones") return <ExpenseRequests />;
  if (path === "/configuracion") return <Settings />;
  if (path === "/gastos") return <Expenses />;
  if (path === "/ingresos") return <Income />;
  if (path === "/ahorros") return <Savings />;
  if (path === "/gastos-futuros") return <FutureExpenses />;
  if (path === "/facturas") return <Invoices />;
  if (path === "/declaracion") return <TaxFiling />;
  if (path === "/pagos-recurrentes") return <Bills />;
  if (path === "/declaraciones-presentadas") return <FiledRecords />;
  if (path === "/cierre-de-mes") return <MonthClose />;
  if (path === "/sistema") return <System />;
  return null;
}

export function App() {
  const { role } = useAuth();
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/verify" element={<VerifyEmail />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          {modules.map((m) => (
            <Route
              key={m.path}
              path={m.path}
              element={canOpen(role, m.path) ? (pageFor(m.path) ?? <Placeholder module={m} />) : <Navigate to="/" replace />}
            />
          ))}
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
