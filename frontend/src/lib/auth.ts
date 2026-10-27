import { api } from "./api";

export type User = {
  id: string;
  google_sub: string;
  email: string;
  name: string;
  picture?: string;
  role: string;
  credits: number;
};

export type MeResponse = { user: User };

export async function getMe(): Promise<User | null> {
  try {
    const data = await api.get<MeResponse>("/api/auth/me");
    return data.user;
  } catch {
    return null;
  }
}

export async function logout(): Promise<void> {
  await api.post("/api/auth/logout");
}

export function isAdmin(role: string | undefined): boolean {
  return role === "admin";
}
