import { createContext, useContext } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { dashboardKeys } from "../../api/dashboard";
import { expensesKeys } from "../../api/expenses";
import {
  addFutureSaving,
  assignFutureSaving,
  createFutureExpense,
  deleteFutureExpense,
  futureExpensesKeys,
  listFutureExpenses,
  payFutureExpense,
  updateFutureExpense,
  type FutureExpenseInput,
  type FutureExpenseItem,
  type FutureExpensesList,
  type PayFutureInput,
  type SavingInput,
} from "../../api/futureExpenses";
import {
  addPersonalFutureSaving,
  assignPersonalFutureSaving,
  createPersonalFutureExpense,
  deletePersonalFutureExpense,
  listPersonalFutureExpenses,
  payPersonalFutureExpense,
  personalKeys,
  updatePersonalFutureExpense,
} from "../../api/personal";
import { savingsKeys } from "../../api/savings";

/** What the payment of an item reports back: the item and the Gasto it registered. */
export interface PaidResult {
  item: FutureExpenseItem;
  expense: { amount: string; category: string };
}

/**
 * The API a future expenses page talks to. The owner's page and her "Mis gastos futuros" page are the same screen
 * over two storages (family ledger vs. her personal ledger), so the components take this from a context instead of
 * importing one client. `pay` receives the category as a string: the owner's is a category name, hers is the id of
 * one of her categories (the adapter converts it).
 */
export interface FutureBackend {
  listKey: readonly unknown[];
  list: () => Promise<FutureExpensesList>;
  create: (input: FutureExpenseInput) => Promise<FutureExpenseItem>;
  update: (id: number, input: FutureExpenseInput) => Promise<FutureExpenseItem>;
  remove: (id: number) => Promise<void>;
  addSaving: (id: number, input: SavingInput) => Promise<unknown>;
  assign: (id: number, input: SavingInput) => Promise<unknown>;
  pay: (id: number, input: PayFutureInput) => Promise<PaidResult>;
  /** Refreshes everything a change can touch. */
  invalidate: (qc: QueryClient) => Promise<unknown>;
}

/** The owner's family ledger. Refreshes the dashboard card, the savings and the expenses a payment touches. */
export const ownerFutureBackend: FutureBackend = {
  listKey: futureExpensesKeys.list,
  list: listFutureExpenses,
  create: createFutureExpense,
  update: updateFutureExpense,
  remove: deleteFutureExpense,
  addSaving: addFutureSaving,
  assign: assignFutureSaving,
  pay: payFutureExpense,
  invalidate: (qc) =>
    Promise.all([
      qc.invalidateQueries({ queryKey: futureExpensesKeys.all }),
      qc.invalidateQueries({ queryKey: dashboardKeys.all }),
      qc.invalidateQueries({ queryKey: savingsKeys.all }),
      qc.invalidateQueries({ queryKey: expensesKeys.all }),
    ]),
};

/** Her personal ledger: every query under ["personal"] (items, her savings and her expenses) is refreshed. */
export const personalFutureBackend: FutureBackend = {
  listKey: personalKeys.futureExpenses,
  list: listPersonalFutureExpenses,
  create: createPersonalFutureExpense,
  update: updatePersonalFutureExpense,
  remove: deletePersonalFutureExpense,
  addSaving: addPersonalFutureSaving,
  assign: assignPersonalFutureSaving,
  pay: (id, input) =>
    payPersonalFutureExpense(id, {
      category_id: Number(input.category),
      ...(input.amount !== undefined ? { amount: input.amount } : {}),
      ...(input.date !== undefined ? { date: input.date } : {}),
    }),
  invalidate: (qc) => qc.invalidateQueries({ queryKey: personalKeys.all }),
};

const FutureBackendContext = createContext<FutureBackend>(ownerFutureBackend);

export const FutureBackendProvider = FutureBackendContext.Provider;
export const useFutureBackend = () => useContext(FutureBackendContext);
