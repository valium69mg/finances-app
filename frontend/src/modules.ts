export interface ModuleTab {
  path: string;
  label: string;
}

// One tab per module in PLAN.md section 4.
export const modules: ModuleTab[] = [
  { path: "/", label: "Panel" },
  { path: "/ingresos", label: "Ingresos" },
  { path: "/gastos", label: "Gastos" },
  { path: "/ahorros", label: "Ahorros" },
  { path: "/facturas", label: "Facturas" },
  { path: "/declaracion", label: "Declaración" },
  { path: "/declaraciones-presentadas", label: "Declaraciones presentadas" },
  { path: "/cierre-de-mes", label: "Cierre de mes" },
  { path: "/pagos-recurrentes", label: "Pagos recurrentes" },
  { path: "/configuracion", label: "Configuración" },
];
