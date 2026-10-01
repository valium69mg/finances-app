/** A cheap shape check (something@something.tld, no spaces) so obvious typos never reach the server, which validates for real. */
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export interface UserErrors {
  email?: string;
}

export function validateNewUser(email: string): UserErrors {
  const value = email.trim();
  if (value === "") return { email: "Escribe el correo de la persona." };
  if (value.length > 254 || !EMAIL.test(value)) return { email: "Escribe un correo electrónico válido." };
  return {};
}
