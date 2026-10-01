import { api } from "./client";
import type { Role } from "./tokens";

/** Users API (owner only): accounts, their state and the invitation email. */

export interface UserAccount {
  id: string;
  email: string;
  role: Role;
  /** An inactive account cannot sign in or refresh. */
  active: boolean;
  /** The person already chose a password through the emailed link. */
  verified: boolean;
  created_at: string;
}

/** Body of POST /users. Only household accounts can be created. */
export interface NewUserInput {
  email: string;
  role: "household";
}

export const usersKeys = {
  all: ["users"] as const,
};

type Client = Pick<typeof api, "request">;

export function createUsersApi(client: Client = api) {
  return {
    list: () => client.request<UserAccount[]>("/users"),
    create: (input: NewUserInput) => client.request<UserAccount>("/users", { method: "POST", body: input }),
    /** Emails a new verification link with the invitation text; calling it again issues a new one. */
    invite: (id: string) => client.request<void>(`/users/${encodeURIComponent(id)}/invite`, { method: "POST" }),
    deactivate: (id: string) => client.request<void>(`/users/${encodeURIComponent(id)}/deactivate`, { method: "POST" }),
    activate: (id: string) => client.request<void>(`/users/${encodeURIComponent(id)}/activate`, { method: "POST" }),
  };
}

const defaultApi = createUsersApi();

export const listUsers = defaultApi.list;
export const createUser = defaultApi.create;
export const inviteUser = defaultApi.invite;
export const deactivateUser = defaultApi.deactivate;
export const activateUser = defaultApi.activate;
