import type { ComponentProps } from "react";
import { TextField } from "../../components/AuthCard";

type Props = Omit<ComponentProps<typeof TextField>, "inputMode" | "suffix">;

/** Text input for a percentage: decimal keyboard and a visible "%" at the end. The label should name the unit. */
export function PercentField(props: Props) {
  return <TextField {...props} inputMode="decimal" suffix="%" autoComplete="off" />;
}
