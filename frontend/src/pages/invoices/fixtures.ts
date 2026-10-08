import type { AllSettings, Client } from "../../api/settings";

const client = (over: Partial<Client>): Client => ({
  id: "",
  name: "",
  type: "",
  currency: "MXN",
  iva_rate: "0",
  rfc: "",
  regimen: "",
  uso_cfdi: "",
  ret_isr_rate: "0",
  ret_iva_rate: "0",
  concepto: "",
  clave_prod_serv: "",
  clave_unidad: "",
  address: "",
  tax_residence: "",
  contract: "",
  real_payer: "",
  postal_code: "",
  ...over,
});

/** Settings with the two clients the invoice tests need (test-only). */
export const SETTINGS: AllSettings = {
  general: {
    salary_usd: "2500.00",
    fx_rate_applied: "17.50",
    morse_fee_rate: "0.01",
    emergency_months: "6",
    extra_income_estimate_mxn: "5000.00",
    budget_includes_extra_income: false,
    extra_income_split: {},
    investment_allocation: [],
    cycle_start_day: 0,
  },
  categories: [],
  clients: [client({ id: "usa", name: "Acme Inc.", currency: "USD" }), client({ id: "b", name: "Público en general", iva_rate: "0.16", rfc: "XAXX010101000" })],
  instruments: { instruments: [], by_category: {} },
  brackets: [],
  payment_methods: [],
  issuer: null,
  investment_pause: null,
};
