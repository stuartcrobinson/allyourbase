/** List response envelope returned by collection endpoints. */
export interface ListResponse<T = Record<string, unknown>> {
  items: T[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

/** Parameters for listing records. */
export interface ListParams {
  page?: number;
  perPage?: number;
  sort?: string;
  filter?: string;
  fields?: string;
  expand?: string;
  skipTotal?: boolean;
}

/** Parameters for reading a single record. */
export interface GetParams {
  fields?: string;
  expand?: string;
}

/** Auth tokens returned by login/register. */
export interface AuthResponse {
  token: string;
  refreshToken: string;
  user: User;
}

/** User record from the auth system. */
export interface User {
  id: string;
  email: string;
  emailVerified?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

/** Realtime event from SSE stream. */
export interface RealtimeEvent {
  action: "create" | "update" | "delete";
  table: string;
  record: Record<string, unknown>;
}

/** Stored file metadata returned by storage endpoints. */
export interface StorageObject {
  id: string;
  bucket: string;
  name: string;
  size: number;
  contentType: string;
  userId?: string;
  createdAt: string;
  updatedAt: string;
}

/** Client configuration options. */
export interface ClientOptions {
  /** Custom fetch implementation (e.g. for Node.js < 18). */
  fetch?: typeof globalThis.fetch;
}
