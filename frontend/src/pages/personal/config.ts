import type { MovementKind } from "../../api/dashboard";

/** Spanish copy of each personal page. */
export interface PersonalPageConfig {
  kind: MovementKind;
  title: string;
  intro: string;
  newLabel: string;
  listTitle: string;
  loading: string;
  empty: string;
  totalLabel: string;
  /** Article + noun used in confirmations and errors, e.g. "el gasto". */
  noun: string;
  deleteQuestion: string;
}

export const PERSONAL_PAGES: Record<MovementKind, PersonalPageConfig> = {
  Ingreso: {
    kind: "Ingreso",
    title: "Mis ingresos",
    intro: "Registra el dinero que recibes: trabajo, transferencias y otros.",
    newLabel: "Nuevo ingreso",
    listTitle: "Ingresos del periodo",
    loading: "Cargando ingresos…",
    empty: "No hay ingresos registrados en este periodo.",
    totalLabel: "Total de ingresos",
    noun: "el ingreso",
    deleteQuestion: "¿Eliminar este ingreso?",
  },
  Gasto: {
    kind: "Gasto",
    title: "Mis gastos",
    intro: "Registra tus gastos personales y organízalos con tus propias categorías.",
    newLabel: "Nuevo gasto",
    listTitle: "Gastos del periodo",
    loading: "Cargando gastos…",
    empty: "No hay gastos registrados en este periodo.",
    totalLabel: "Total de gastos",
    noun: "el gasto",
    deleteQuestion: "¿Eliminar este gasto?",
  },
  Ahorro: {
    kind: "Ahorro",
    title: "Mis ahorros",
    intro: "Registra tus aportaciones y retiros de ahorro e inversión.",
    newLabel: "Nuevo movimiento",
    listTitle: "Ahorros del periodo",
    loading: "Cargando ahorros…",
    empty: "No hay aportaciones ni retiros en este periodo.",
    totalLabel: "Ahorro neto del periodo",
    noun: "el movimiento",
    deleteQuestion: "¿Eliminar este movimiento?",
  },
};
