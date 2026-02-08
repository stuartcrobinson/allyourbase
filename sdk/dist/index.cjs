"use strict";
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/index.ts
var index_exports = {};
__export(index_exports, {
  AYBClient: () => AYBClient,
  AYBError: () => AYBError
});
module.exports = __toCommonJS(index_exports);

// src/errors.ts
var AYBError = class extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
    this.name = "AYBError";
  }
};

// src/client.ts
var AYBClient = class {
  constructor(baseURL, options) {
    this._token = null;
    this._refreshToken = null;
    this.baseURL = baseURL.replace(/\/+$/, "");
    this._fetch = options?.fetch ?? globalThis.fetch.bind(globalThis);
    this.auth = new AuthClient(this);
    this.records = new RecordsClient(this);
    this.storage = new StorageClient(this);
    this.realtime = new RealtimeClient(this);
  }
  /** Current access token, if authenticated. */
  get token() {
    return this._token;
  }
  /** Current refresh token, if authenticated. */
  get refreshToken() {
    return this._refreshToken;
  }
  /** Manually set auth tokens (e.g. from storage). */
  setTokens(token, refreshToken) {
    this._token = token;
    this._refreshToken = refreshToken;
  }
  /** Clear stored auth tokens. */
  clearTokens() {
    this._token = null;
    this._refreshToken = null;
  }
  /** @internal */
  async request(path, init) {
    const headers = {
      ...init?.headers
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
    if (res.status === 204) return void 0;
    return res.json();
  }
  /** @internal */
  setTokensInternal(token, refreshToken) {
    this._token = token;
    this._refreshToken = refreshToken;
  }
  /** @internal */
  getBaseURL() {
    return this.baseURL;
  }
};
var AuthClient = class {
  constructor(client) {
    this.client = client;
  }
  /** Register a new user account. */
  async register(email, password) {
    const res = await this.client.request("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password })
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    return res;
  }
  /** Log in with email and password. */
  async login(email, password) {
    const res = await this.client.request("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password })
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    return res;
  }
  /** Refresh the access token using the stored refresh token. */
  async refresh() {
    const res = await this.client.request("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken })
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    return res;
  }
  /** Log out (revoke the refresh token). */
  async logout() {
    await this.client.request("/api/auth/logout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken })
    });
    this.client.clearTokens();
  }
  /** Get the current authenticated user. */
  async me() {
    return this.client.request("/api/auth/me");
  }
  /** Request a password reset email. */
  async requestPasswordReset(email) {
    await this.client.request("/api/auth/password-reset", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email })
    });
  }
  /** Confirm a password reset with a token. */
  async confirmPasswordReset(token, password) {
    await this.client.request("/api/auth/password-reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, password })
    });
  }
  /** Verify an email address with a token. */
  async verifyEmail(token) {
    await this.client.request("/api/auth/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token })
    });
  }
  /** Resend the email verification (requires auth). */
  async resendVerification() {
    await this.client.request("/api/auth/verify/resend", {
      method: "POST",
      headers: { "Content-Type": "application/json" }
    });
  }
};
var RecordsClient = class {
  constructor(client) {
    this.client = client;
  }
  /** List records in a collection with optional filtering, sorting, and pagination. */
  async list(collection, params) {
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
  async get(collection, id, params) {
    const qs = new URLSearchParams();
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${collection}/${id}${suffix}`);
  }
  /** Create a new record. */
  async create(collection, data) {
    return this.client.request(`/api/collections/${collection}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data)
    });
  }
  /** Update an existing record (partial update). */
  async update(collection, id, data) {
    return this.client.request(`/api/collections/${collection}/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data)
    });
  }
  /** Delete a record by primary key. */
  async delete(collection, id) {
    return this.client.request(`/api/collections/${collection}/${id}`, {
      method: "DELETE"
    });
  }
};
var StorageClient = class {
  constructor(client) {
    this.client = client;
  }
  /** Upload a file to a bucket. */
  async upload(bucket, file, name) {
    const form = new FormData();
    form.append("file", file, name ?? (file instanceof File ? file.name : "upload"));
    return this.client.request(`/api/storage/${bucket}`, {
      method: "POST",
      body: form
    });
  }
  /** Get a download URL for a file. */
  downloadURL(bucket, name) {
    return `${this.client.getBaseURL()}/api/storage/${bucket}/${name}`;
  }
  /** Delete a file from a bucket. */
  async delete(bucket, name) {
    return this.client.request(`/api/storage/${bucket}/${name}`, {
      method: "DELETE"
    });
  }
  /** List files in a bucket. */
  async list(bucket, params) {
    const qs = new URLSearchParams();
    if (params?.prefix) qs.set("prefix", params.prefix);
    if (params?.limit) qs.set("limit", String(params.limit));
    if (params?.offset) qs.set("offset", String(params.offset));
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/storage/${bucket}${suffix}`);
  }
  /** Get a signed URL for time-limited access to a file. */
  async getSignedURL(bucket, name, expiresIn) {
    return this.client.request(`/api/storage/${bucket}/${name}/sign`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expiresIn: expiresIn ?? 3600 })
    });
  }
};
var RealtimeClient = class {
  constructor(client) {
    this.client = client;
  }
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
  subscribe(tables, callback) {
    const params = new URLSearchParams({ tables: tables.join(",") });
    if (this.client.token) {
      params.set("token", this.client.token);
    }
    const url = `${this.client.getBaseURL()}/api/realtime?${params}`;
    const es = new EventSource(url);
    es.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data);
        callback(event);
      } catch {
      }
    };
    return () => es.close();
  }
};
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  AYBClient,
  AYBError
});
//# sourceMappingURL=index.cjs.map