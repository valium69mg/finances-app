import type { LucideIcon } from "lucide-react";
import { TONES, type Tone } from "../lib/tones";

const SIZES = {
  sm: { box: "h-8 w-8", icon: "h-4 w-4" }, // 16px glyph
  md: { box: "h-10 w-10", icon: "h-5 w-5" }, // 20px glyph
  lg: { box: "h-12 w-12", icon: "h-6 w-6" }, // 24px glyph
} as const;

interface Props {
  icon: LucideIcon;
  tone?: Tone;
  size?: keyof typeof SIZES;
  /** The chip sits on a soft-tinted card, so it uses the plain surface instead of the tint. */
  onSoft?: boolean;
}

/** Decorative rounded square holding one icon in a tone. Always paired with visible text, so it is hidden from assistive tech. */
export function IconChip({ icon: Icon, tone = "primary", size = "md", onSoft = false }: Props) {
  const s = SIZES[size];
  const t = TONES[tone];
  return (
    <span aria-hidden="true" className={`inline-flex shrink-0 items-center justify-center rounded-lg ${s.box} ${onSoft ? t.chipOnSoft : t.chip}`}>
      <Icon className={s.icon} />
    </span>
  );
}
