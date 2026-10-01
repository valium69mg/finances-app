import { ArrowDownLeft, ArrowUpRight, PiggyBank, type LucideIcon } from "lucide-react";
import type { MovementKind } from "../api/dashboard";

/** Semantic color family. Class names are written out in full so Tailwind can see them. */
export type Tone = "primary" | "income" | "expense" | "saving";

interface ToneClasses {
  /** Strong color for icons and text. */
  text: string;
  /** Soft tint background with the strong color on it (chips, badges). */
  chip: string;
  /** Same chip when it sits on a soft-tinted card: the surface keeps it apart from the card. */
  chipOnSoft: string;
  /** Soft tinted card: background and a faint border of the same family. */
  card: string;
  /** Progress bar fill. */
  bar: string;
}

export const TONES: Record<Tone, ToneClasses> = {
  primary: { text: "text-primary", chip: "bg-primary/10 text-primary", chipOnSoft: "bg-surface text-primary", card: "border-border bg-surface", bar: "bg-primary" },
  income: { text: "text-income", chip: "bg-income-soft text-income", chipOnSoft: "bg-surface text-income", card: "border-income/25 bg-income-soft", bar: "bg-income" },
  expense: { text: "text-expense", chip: "bg-expense-soft text-expense", chipOnSoft: "bg-surface text-expense", card: "border-expense/25 bg-expense-soft", bar: "bg-expense" },
  saving: { text: "text-saving", chip: "bg-saving-soft text-saving", chipOnSoft: "bg-surface text-saving", card: "border-saving/25 bg-saving-soft", bar: "bg-saving" },
};

/** Icon and tone of each movement kind (the kind is always also written out next to them). */
export const KIND: Record<MovementKind, { icon: LucideIcon; tone: Tone }> = {
  Ingreso: { icon: ArrowDownLeft, tone: "income" },
  Gasto: { icon: ArrowUpRight, tone: "expense" },
  Ahorro: { icon: PiggyBank, tone: "saving" },
};
