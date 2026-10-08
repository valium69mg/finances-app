import type { Bracket, Category, Client, General, Instrument, Pause } from "../../api/settings";
import { distribution, isAtMostHundred, isBudgetKind, isHundred, isPlainDecimal, percentToRate, rateToPercent, sumDecimals } from "../../lib/planning";
import { formatMoney } from "../expenses/money";

/** Field-level errors keyed by a path such as "salary_usd" or "3.budget". */
export type Errors = Record<string, string>;

const DECIMAL = /^\d+(\.\d+)?$/;
const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;

export const isDecimal = (s: string) => DECIMAL.test(s.trim());
export const isMonth = (s: string) => MONTH.test(s.trim());

const MSG = {
  required: "Este campo es obligatorio.",
  decimal: "Escribe un número válido, sin signos ni comas (por ejemplo 1500.50).",
  month: "Usa el formato AAAA-MM (por ejemplo 2026-10).",
  postalCode: "El código postal debe tener 5 dígitos o dejarse vacío.",
  cycleStartDay: "Elige cuándo empieza tu periodo.",
  percent: "Escribe un porcentaje válido, sin signos ni comas (por ejemplo 16.5).",
  percentRange: "El porcentaje debe estar entre 0 y 100.",
  splitSum: (sum: string) => `Fondo de emergencia, inversiones y aguinaldo y vacaciones deben sumar exactamente 100 %. Ahora suman ${sum} %.`,
  allocSum: (sum: string) => `La distribución de inversiones debe sumar exactamente 100 %. Ahora suma ${sum} %.`,
  budgetTotal: (assigned: string, excess: string, base: string) =>
    `Los presupuestos suman ${formatMoney(assigned)} y exceden por ${formatMoney(excess)} el ingreso base de ${formatMoney(base)} (100 %). Reduce algún presupuesto para guardar.`,
};

function decimal(errors: Errors, key: string, value: string, required = true) {
  const v = value.trim();
  if (!v) {
    if (required) errors[key] = MSG.required;
  } else if (!isDecimal(v)) {
    errors[key] = MSG.decimal;
  }
}

function text(errors: Errors, key: string, value: string) {
  if (!value.trim()) errors[key] = MSG.required;
}

/**
 * General section as edited in the form: it has the shape of the API payload, but
 * the extra-income split and the investment allocation hold percentages ("16.5")
 * instead of decimals ("0.165"). They are converted at the edge, exactly.
 */
export type GeneralDraft = General;

/** Extra-income parts that must add up to 100% (the SAT reserve is taken off before and stays separate). */
export const SPLIT_DESTINATIONS = ["fondo_emergencia", "inversiones", "aguinaldo_vacaciones"] as const;
const SPLIT_DEFAULTS: Record<string, string> = { fondo_emergencia: "50", inversiones: "35", aguinaldo_vacaciones: "15" };

export const toGeneralDraft = (g: General): GeneralDraft => ({
  ...g,
  extra_income_split: Object.fromEntries(Object.entries(g.extra_income_split).map(([k, v]) => [k, rateToPercent(v)])),
  investment_allocation: g.investment_allocation.map((w) => ({ ...w, value: rateToPercent(w.value) })),
});

export const fromGeneralDraft = (d: GeneralDraft): General => ({
  ...d,
  extra_income_split: Object.fromEntries(Object.entries(d.extra_income_split).map(([k, v]) => [k, percentToRate(v.trim())])),
  investment_allocation: d.investment_allocation.map((w) => ({ ...w, value: percentToRate(w.value.trim()) })),
});

/** Sum of the three split destinations in percent; absent ones count at their default, as the API does. Null when the split has none of them. */
export function splitDestinationsTotal(split: Record<string, string>): string | null {
  if (!SPLIT_DESTINATIONS.some((k) => k in split)) return null;
  return sumDecimals(SPLIT_DESTINATIONS.map((k) => (k in split ? split[k] : SPLIT_DEFAULTS[k])));
}

/** Sum of the investment allocation in percent; null when there is none. */
export function allocationTotal(alloc: { value: string }[]): string | null {
  return alloc.length === 0 ? null : sumDecimals(alloc.map((w) => w.value));
}

function percent(errors: Errors, key: string, value: string) {
  const v = value.trim();
  if (!v) errors[key] = MSG.required;
  else if (!isPlainDecimal(v)) errors[key] = MSG.percent;
  else if (!isAtMostHundred(v)) errors[key] = MSG.percentRange;
}

export function validateGeneral(g: GeneralDraft): Errors {
  const e: Errors = {};
  decimal(e, "salary_usd", g.salary_usd);
  decimal(e, "fx_rate_applied", g.fx_rate_applied);
  decimal(e, "morse_fee_rate", g.morse_fee_rate);
  decimal(e, "emergency_months", g.emergency_months);
  decimal(e, "extra_income_estimate_mxn", g.extra_income_estimate_mxn);
  if (!Number.isInteger(g.cycle_start_day) || g.cycle_start_day < 0 || g.cycle_start_day > 31) e["cycle_start_day"] = MSG.cycleStartDay;
  for (const [k, v] of Object.entries(g.extra_income_split)) percent(e, `split.${k}`, v);
  g.investment_allocation.forEach((w, i) => percent(e, `alloc.${i}`, w.value));
  const splitOk = !Object.keys(e).some((k) => k.startsWith("split."));
  const splitTotal = splitDestinationsTotal(g.extra_income_split);
  if (splitOk && splitTotal !== null && !isHundred(splitTotal)) e["split"] = MSG.splitSum(splitTotal);
  const allocOk = !Object.keys(e).some((k) => k.startsWith("alloc."));
  const allocSum = allocationTotal(g.investment_allocation);
  if (allocOk && allocSum !== null && !isHundred(allocSum)) e["alloc"] = MSG.allocSum(allocSum);
  return e;
}

/** Category as edited in the form: budget and keywords stay raw text until saved. */
export interface CategoryDraft {
  name: string;
  kind: string;
  budget: string;
  includes: string;
  keywords: string;
}

export const toCategoryDraft = (c: Category): CategoryDraft => ({
  name: c.name,
  kind: c.kind,
  budget: c.budget ?? "",
  includes: c.includes,
  keywords: c.keywords.join(", "),
});

export const fromCategoryDraft = (d: CategoryDraft): Category => ({
  name: d.name,
  kind: d.kind,
  budget: d.budget.trim() === "" ? null : d.budget.trim(),
  includes: d.includes,
  keywords: d.keywords
    .split(",")
    .map((k) => k.trim())
    .filter(Boolean),
});

/**
 * Budgets must be decimals and, when the base monthly income is available, the
 * Gasto and Ahorro budgets may not add up to more than it ("total" error). Without
 * a base (null) only the amounts are checked.
 */
export function validateCategories(cats: CategoryDraft[], base: string | null = null): Errors {
  const e: Errors = {};
  cats.forEach((c, i) => decimal(e, `${i}.budget`, c.budget, false));
  if (base !== null && Object.keys(e).length === 0) {
    const d = distribution(cats.filter((c) => isBudgetKind(c.kind)), base);
    if (d?.over) e["total"] = MSG.budgetTotal(d.assigned, d.excess, d.base);
  }
  return e;
}

export function validateClients(clients: Client[]): Errors {
  const e: Errors = {};
  clients.forEach((c, i) => {
    text(e, `${i}.name`, c.name);
    text(e, `${i}.currency`, c.currency);
    if (c.postal_code.trim() && !/^\d{5}$/.test(c.postal_code.trim())) e[`${i}.postal_code`] = MSG.postalCode;
    decimal(e, `${i}.iva_rate`, c.iva_rate);
    decimal(e, `${i}.ret_isr_rate`, c.ret_isr_rate);
    decimal(e, `${i}.ret_iva_rate`, c.ret_iva_rate);
  });
  return e;
}

export function validateInstruments(list: Instrument[]): Errors {
  const e: Errors = {};
  list.forEach((c, i) => text(e, `${i}.name`, c.name));
  return e;
}

export function validateBrackets(rows: Bracket[]): Errors {
  const e: Errors = {};
  if (rows.length === 0) e["rows"] = "Agrega al menos un rango.";
  rows.forEach((r, i) => {
    decimal(e, `${i}.upper`, r.upper);
    decimal(e, `${i}.rate`, r.rate);
  });
  return e;
}

export interface PauseDraft {
  monthsText: string;
  normal_budget: string;
  resume_month: string;
  plan: Record<string, string>;
  note: string;
}

export const parseMonths = (text: string): string[] =>
  text
    .split(/[,\s]+/)
    .map((m) => m.trim())
    .filter(Boolean);

export const toPauseDraft = (p: Pause | null): PauseDraft => ({
  monthsText: p ? p.months.join(", ") : "",
  normal_budget: p?.normal_budget ?? "",
  resume_month: p?.resume_month ?? "",
  plan: p ? { ...p.future_expenses_plan } : {},
  note: p?.note ?? "",
});

/** Only the plan entries for months that are still paused are sent. */
export const fromPauseDraft = (d: PauseDraft): Pause => {
  const months = parseMonths(d.monthsText);
  const plan: Record<string, string> = {};
  for (const m of months) plan[m] = (d.plan[m] ?? "").trim();
  return { months, normal_budget: d.normal_budget.trim(), resume_month: d.resume_month.trim(), future_expenses_plan: plan, note: d.note };
};

export function validatePause(d: PauseDraft): Errors {
  const e: Errors = {};
  const months = parseMonths(d.monthsText);
  if (months.length === 0) e["months"] = MSG.required;
  else if (!months.every(isMonth)) e["months"] = MSG.month;
  else if (new Set(months).size !== months.length) e["months"] = "Hay meses repetidos.";
  decimal(e, "normal_budget", d.normal_budget);
  if (!d.resume_month.trim()) e["resume_month"] = MSG.required;
  else if (!isMonth(d.resume_month)) e["resume_month"] = MSG.month;
  if (!e["months"]) {
    for (const m of months) decimal(e, `plan.${m}`, d.plan[m] ?? "");
  }
  return e;
}
