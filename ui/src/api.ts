import type { SchemaCache, ListResponse } from "./types";

const TOKEN_KEY = "ayb_admin_token";

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    ...(init?.headers as Record<string, string>),
  };
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
  return res.json();
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function getAdminStatus(): Promise<{ auth: boolean }> {
  return request("/api/admin/status");
}

export async function adminLogin(password: string): Promise<string> {
  const res = await request<{ token: string }>("/api/admin/auth", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
  setToken(res.token);
  return res.token;
}

export async function getSchema(): Promise<SchemaCache> {
  return request("/api/schema");
}

export async function getRows(
  table: string,
  params: {
    page?: number;
    perPage?: number;
    sort?: string;
    filter?: string;
  } = {},
): Promise<ListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  if (params.sort) qs.set("sort", params.sort);
  if (params.filter) qs.set("filter", params.filter);
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/collections/${table}${suffix}`);
}

export async function createRecord(
  table: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  return request(`/api/collections/${table}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateRecord(
  table: string,
  id: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  return request(`/api/collections/${table}/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteRecord(
  table: string,
  id: string,
): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`/api/collections/${table}/${id}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message || res.statusText);
  }
}
