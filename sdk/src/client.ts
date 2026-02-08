import { AYBError } from "./errors";
import type {
  AuthResponse,
  ClientOptions,
  GetParams,
  ListParams,
  ListResponse,
  RealtimeEvent,
  StorageObject,
  User,
} from "./types";

/**
 * AllYourBase JavaScript/TypeScript client.
 *
 * @example
 * ```ts
 * import { AYBClient } from "@allyourbase/js";
 *
 * const ayb = new AYBClient("http://localhost:8090");
 *
 * // List records
 * const posts = await ayb.records.list("posts", { filter: "published=true", sort: "-created_at" });
 *
 * // Auth
 * await ayb.auth.login("user@example.com", "password");
 * const me = await ayb.auth.me();
 * ```
 */
export class AYBClient {
  private baseURL: string;
  private _fetch: typeof globalThis.fetch;
  private _token: string | null = null;
  private _refreshToken: string | null = null;

  readonly auth: AuthClient;
  readonly records: RecordsClient;
  readonly storage: StorageClient;
  readonly realtime: RealtimeClient;

  constructor(baseURL: string, options?: ClientOptions) {
    // Strip trailing slash.
    this.baseURL = baseURL.replace(/\/+$/, "");
    this._fetch = options?.fetch ?? globalThis.fetch.bind(globalThis);

    this.auth = new AuthClient(this);
    this.records = new RecordsClient(this);
    this.storage = new StorageClient(this);
    this.realtime = new RealtimeClient(this);
  }

  /** Current access token, if authenticated. */
  get token(): string | null {
    return this._token;
  }

  /** Current refresh token, if authenticated. */
  get refreshToken(): string | null {
    return this._refreshToken;
  }

  /** Manually set auth tokens (e.g. from storage). */
  setTokens(token: string, refreshToken: string): void {
    this._token = token;
    this._refreshToken = refreshToken;
  }

  /** Clear stored auth tokens. */
  clearTokens(): void {
    this._token = null;
    this._refreshToken = null;
  }

  /** @internal */
  async request<T>(
    path: string,
    init?: RequestInit & { skipAuth?: boolean },
  ): Promise<T> {
    const headers: Record<string, string> = {
      ...(init?.headers as Record<string, string>),
    };
    if (!init?.skipAuth && this._token) {
      headers["Authorization"] = `Bearer ${this._token}`;
    }
    const url = `${this.baseURL}${path}`;
    const res = await this._fetch(url, { ...init, headers });
    if (!res.ok) {
      const body = await res.json().catch(() => ({ message: res.statusText }));
      throw new AYBError(res.status, body.message || res.statusText);
    }
    // Handle 204 No Content.
    if (res.status === 204) return undefined as T;
    return res.json();
  }

  /** @internal */
  setTokensInternal(token: string, refreshToken: string): void {
    this._token = token;
    this._refreshToken = refreshToken;
  }

  /** @internal */
  getBaseURL(): string {
    return this.baseURL;
  }
}

class AuthClient {
  constructor(private client: AYBClient) {}

  /** Register a new user account. */
  async register(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    return res;
  }

  /** Log in with email and password. */
  async login(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    return res;
  }

  /** Refresh the access token using the stored refresh token. */
  async refresh(): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    return res;
  }

  /** Log out (revoke the refresh token). */
  async logout(): Promise<void> {
    await this.client.request<void>("/api/auth/logout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    this.client.clearTokens();
  }

  /** Get the current authenticated user. */
  async me(): Promise<User> {
    return this.client.request<User>("/api/auth/me");
  }

  /** Request a password reset email. */
  async requestPasswordReset(email: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
  }

  /** Confirm a password reset with a token. */
  async confirmPasswordReset(token: string, password: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, password }),
    });
  }

  /** Verify an email address with a token. */
  async verifyEmail(token: string): Promise<void> {
    await this.client.request<void>("/api/auth/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
  }

  /** Resend the email verification (requires auth). */
  async resendVerification(): Promise<void> {
    await this.client.request<void>("/api/auth/verify/resend", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
  }
}

class RecordsClient {
  constructor(private client: AYBClient) {}

  /** List records in a collection with optional filtering, sorting, and pagination. */
  async list<T = Record<string, unknown>>(
    collection: string,
    params?: ListParams,
  ): Promise<ListResponse<T>> {
    const qs = new URLSearchParams();
    if (params?.page) qs.set("page", String(params.page));
    if (params?.perPage) qs.set("perPage", String(params.perPage));
    if (params?.sort) qs.set("sort", params.sort);
    if (params?.filter) qs.set("filter", params.filter);
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    if (params?.skipTotal) qs.set("skipTotal", "true");
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${collection}${suffix}`);
  }

  /** Get a single record by primary key. */
  async get<T = Record<string, unknown>>(
    collection: string,
    id: string,
    params?: GetParams,
  ): Promise<T> {
    const qs = new URLSearchParams();
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${collection}/${id}${suffix}`);
  }

  /** Create a new record. */
  async create<T = Record<string, unknown>>(
    collection: string,
    data: Record<string, unknown>,
  ): Promise<T> {
    return this.client.request(`/api/collections/${collection}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  /** Update an existing record (partial update). */
  async update<T = Record<string, unknown>>(
    collection: string,
    id: string,
    data: Record<string, unknown>,
  ): Promise<T> {
    return this.client.request(`/api/collections/${collection}/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  /** Delete a record by primary key. */
  async delete(collection: string, id: string): Promise<void> {
    return this.client.request(`/api/collections/${collection}/${id}`, {
      method: "DELETE",
    });
  }
}

class StorageClient {
  constructor(private client: AYBClient) {}

  /** Upload a file to a bucket. */
  async upload(
    bucket: string,
    file: Blob | File,
    name?: string,
  ): Promise<StorageObject> {
    const form = new FormData();
    form.append("file", file, name ?? (file instanceof File ? file.name : "upload"));
    // Don't set Content-Type — the browser/runtime will set it with the boundary.
    return this.client.request(`/api/storage/${bucket}`, {
      method: "POST",
      body: form,
    });
  }

  /** Get a download URL for a file. */
  downloadURL(bucket: string, name: string): string {
    return `${this.client.getBaseURL()}/api/storage/${bucket}/${name}`;
  }

  /** Delete a file from a bucket. */
  async delete(bucket: string, name: string): Promise<void> {
    return this.client.request(`/api/storage/${bucket}/${name}`, {
      method: "DELETE",
    });
  }

  /** List files in a bucket. */
  async list(
    bucket: string,
    params?: { prefix?: string; limit?: number; offset?: number },
  ): Promise<{ items: StorageObject[]; totalItems: number }> {
    const qs = new URLSearchParams();
    if (params?.prefix) qs.set("prefix", params.prefix);
    if (params?.limit) qs.set("limit", String(params.limit));
    if (params?.offset) qs.set("offset", String(params.offset));
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/storage/${bucket}${suffix}`);
  }

  /** Get a signed URL for time-limited access to a file. */
  async getSignedURL(
    bucket: string,
    name: string,
    expiresIn?: number,
  ): Promise<{ url: string }> {
    return this.client.request(`/api/storage/${bucket}/${name}/sign`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expiresIn: expiresIn ?? 3600 }),
    });
  }
}

class RealtimeClient {
  constructor(private client: AYBClient) {}

  /**
   * Subscribe to realtime events for the given tables.
   * Returns an unsubscribe function.
   *
   * @example
   * ```ts
   * const unsub = ayb.realtime.subscribe(["posts", "comments"], (event) => {
   *   console.log(event.action, event.table, event.record);
   * });
   * // Later: unsub();
   * ```
   */
  subscribe(
    tables: string[],
    callback: (event: RealtimeEvent) => void,
  ): () => void {
    const params = new URLSearchParams({ tables: tables.join(",") });
    if (this.client.token) {
      params.set("token", this.client.token);
    }
    const url = `${this.client.getBaseURL()}/api/realtime?${params}`;
    const es = new EventSource(url);

    es.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data) as RealtimeEvent;
        callback(event);
      } catch {
        // Ignore parse errors for heartbeat/ping messages.
      }
    };

    return () => es.close();
  }
}
