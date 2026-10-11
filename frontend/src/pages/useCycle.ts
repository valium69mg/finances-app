import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getSettings, settingsKeys } from "../api/settings";
import { currentCycle, cycleRangeLabel, previousCycle } from "./cycle";
import { useMonthParam } from "../hooks/useUrlParams";
import { todayISO } from "./expenses/money";

/** Cycle start day from the cached settings; 0 (calendar month) until they load or when they fail. */
export function useCycleStartDay(): { startDay: number; ready: boolean } {
  const settings = useQuery({ queryKey: settingsKeys.all, queryFn: getSettings, retry: false });
  return { startDay: settings.data?.general.cycle_start_day ?? 0, ready: !settings.isPending };
}

/**
 * Selected period (YYYY-MM cycle label) of a page. It defaults to the current
 * cycle (or the previous one) and stays null while the settings are loading, so
 * the page never queries or shows a wrong period first. When the settings fail
 * it falls back to the calendar month.
 */
export function useCyclePeriod(initial: "current" | "previous" = "current") {
  const { startDay, ready } = useCycleStartDay();
  const [picked, setPicked] = useState<string | null>(null);
  const current = currentCycle(todayISO(), startDay);
  const fallback = initial === "previous" ? previousCycle(current) : current;
  const month = picked ?? (ready ? fallback : null);
  return {
    month,
    setMonth: setPicked,
    startDay,
    currentMonth: current,
    rangeHint: month ? cycleRangeLabel(month, startDay) : "",
  };
}

/**
 * Like `useCyclePeriod("current")`, but the month lives in the URL (`?mes=YYYY-MM`): the Panel's links open the page
 * on it, picking another month replaces the history entry and keeps the other parameters, and a malformed value is
 * ignored (current cycle). Used by the owner's Gastos, Ingresos and Ahorros.
 */
export function useUrlCyclePeriod() {
  const { startDay, ready } = useCycleStartDay();
  const [picked, setPicked] = useMonthParam();
  const current = currentCycle(todayISO(), startDay);
  const month = picked || (ready ? current : null);
  return {
    month,
    setMonth: setPicked,
    startDay,
    currentMonth: current,
    rangeHint: month ? cycleRangeLabel(month, startDay) : "",
  };
}
