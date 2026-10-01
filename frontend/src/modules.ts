import {
  Activity,
  CalendarCheck,
  Calculator,
  FileCheck,
  FileText,
  LayoutDashboard,
  PiggyBank,
  Receipt,
  Repeat,
  Settings,
  Target,
  TrendingUp,
  type LucideIcon,
} from "lucide-react";

export interface ModuleTab {
  path: string;
  label: string;
  icon: LucideIcon;
  /** Spanish copy shown in the module's empty state. */
  description: string;
}

// One tab per module in PLAN.md section 4.
export const modules: ModuleTab[] = [
  { path: "/", label: "Panel", icon: LayoutDashboard, description: "Aquí verás un resumen de tus finanzas del mes." },
  { path: "/ingresos", label: "Ingresos", icon: TrendingUp, description: "Aquí registrarás y consultarás tus ingresos." },
  { path: "/gastos", label: "Gastos", icon: Receipt, description: "Aquí registrarás y clasificarás tus gastos." },
  { path: "/ahorros", label: "Ahorros", icon: PiggyBank, description: "Aquí darás seguimiento a tus aportaciones de ahorro." },
  { path: "/gastos-futuros", label: "Gastos futuros", icon: Target, description: "Aquí administrarás lo que quieres juntar para gastos futuros." },
  { path: "/facturas", label: "Facturas", icon: FileText, description: "Aquí prepararás y consultarás tus facturas." },
  { path: "/declaracion", label: "Declaración", icon: Calculator, description: "Aquí calcularás tu declaración mensual." },
  {
    path: "/declaraciones-presentadas",
    label: "Declaraciones presentadas",
    icon: FileCheck,
    description: "Aquí verás el historial de declaraciones que ya presentaste.",
  },
  { path: "/cierre-de-mes", label: "Cierre de mes", icon: CalendarCheck, description: "Aquí generarás el cierre de cada mes." },
  { path: "/pagos-recurrentes", label: "Pagos recurrentes", icon: Repeat, description: "Aquí administrarás tus pagos recurrentes." },
  { path: "/sistema", label: "Sistema", icon: Activity, description: "Aquí verás el uso actual del servidor: procesador, memoria y disco." },
  { path: "/configuracion", label: "Configuración", icon: Settings, description: "Aquí ajustarás las preferencias de tu cuenta." },
];

/** Navigation icon of a module, reused by its page header. */
export const moduleIcon = (path: string): LucideIcon => modules.find((m) => m.path === path)?.icon ?? LayoutDashboard;
