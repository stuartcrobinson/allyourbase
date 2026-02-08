/** List response envelope returned by collection endpoints. */
interface ListResponse<T = Record<string, unknown>> {
    items: T[];
    page: number;
    perPage: number;
    totalItems: number;
    totalPages: number;
}
/** Parameters for listing records. */
interface ListParams {
    page?: number;
    perPage?: number;
    sort?: string;
    filter?: string;
    fields?: string;
    expand?: string;
    skipTotal?: boolean;
}
/** Parameters for reading a single record. */
interface GetParams {
    fields?: string;
    expand?: string;
}
/** Auth tokens returned by login/register. */
interface AuthResponse {
    token: string;
    refreshToken: string;
    user: User;
}
/** User record from the auth system. */
interface User {
    id: string;
    email: string;
    emailVerified?: boolean;
    createdAt?: string;
    updatedAt?: string;
}
/** Realtime event from SSE stream. */
interface RealtimeEvent {
    action: "create" | "update" | "delete";
    table: string;
    record: Record<string, unknown>;
}
/** Stored file metadata returned by storage endpoints. */
interface StorageObject {
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
interface ClientOptions {
    /** Custom fetch implementation (e.g. for Node.js < 18). */
    fetch?: typeof globalThis.fetch;
}

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
declare class AYBClient {
    private baseURL;
    private _fetch;
    private _token;
    private _refreshToken;
    readonly auth: AuthClient;
    readonly records: RecordsClient;
    readonly storage: StorageClient;
    readonly realtime: RealtimeClient;
    constructor(baseURL: string, options?: ClientOptions);
    /** Current access token, if authenticated. */
    get token(): string | null;
    /** Current refresh token, if authenticated. */
    get refreshToken(): string | null;
    /** Manually set auth tokens (e.g. from storage). */
    setTokens(token: string, refreshToken: string): void;
    /** Clear stored auth tokens. */
    clearTokens(): void;
    /** @internal */
    request<T>(path: string, init?: RequestInit & {
        skipAuth?: boolean;
    }): Promise<T>;
    /** @internal */
    setTokensInternal(token: string, refreshToken: string): void;
    /** @internal */
    getBaseURL(): string;
}
declare class AuthClient {
    private client;
    constructor(client: AYBClient);
    /** Register a new user account. */
    register(email: string, password: string): Promise<AuthResponse>;
    /** Log in with email and password. */
    login(email: string, password: string): Promise<AuthResponse>;
    /** Refresh the access token using the stored refresh token. */
    refresh(): Promise<AuthResponse>;
    /** Log out (revoke the refresh token). */
    logout(): Promise<void>;
    /** Get the current authenticated user. */
    me(): Promise<User>;
    /** Request a password reset email. */
    requestPasswordReset(email: string): Promise<void>;
    /** Confirm a password reset with a token. */
    confirmPasswordReset(token: string, password: string): Promise<void>;
    /** Verify an email address with a token. */
    verifyEmail(token: string): Promise<void>;
    /** Resend the email verification (requires auth). */
    resendVerification(): Promise<void>;
}
declare class RecordsClient {
    private client;
    constructor(client: AYBClient);
    /** List records in a collection with optional filtering, sorting, and pagination. */
    list<T = Record<string, unknown>>(collection: string, params?: ListParams): Promise<ListResponse<T>>;
    /** Get a single record by primary key. */
    get<T = Record<string, unknown>>(collection: string, id: string, params?: GetParams): Promise<T>;
    /** Create a new record. */
    create<T = Record<string, unknown>>(collection: string, data: Record<string, unknown>): Promise<T>;
    /** Update an existing record (partial update). */
    update<T = Record<string, unknown>>(collection: string, id: string, data: Record<string, unknown>): Promise<T>;
    /** Delete a record by primary key. */
    delete(collection: string, id: string): Promise<void>;
}
declare class StorageClient {
    private client;
    constructor(client: AYBClient);
    /** Upload a file to a bucket. */
    upload(bucket: string, file: Blob | File, name?: string): Promise<StorageObject>;
    /** Get a download URL for a file. */
    downloadURL(bucket: string, name: string): string;
    /** Delete a file from a bucket. */
    delete(bucket: string, name: string): Promise<void>;
    /** List files in a bucket. */
    list(bucket: string, params?: {
        prefix?: string;
        limit?: number;
        offset?: number;
    }): Promise<{
        items: StorageObject[];
        totalItems: number;
    }>;
    /** Get a signed URL for time-limited access to a file. */
    getSignedURL(bucket: string, name: string, expiresIn?: number): Promise<{
        url: string;
    }>;
}
declare class RealtimeClient {
    private client;
    constructor(client: AYBClient);
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
    subscribe(tables: string[], callback: (event: RealtimeEvent) => void): () => void;
}

/** Error thrown when the AYB API returns a non-2xx response. */
declare class AYBError extends Error {
    readonly status: number;
    constructor(status: number, message: string);
}

export { AYBClient, AYBError, type AuthResponse, type ClientOptions, type GetParams, type ListParams, type ListResponse, type RealtimeEvent, type StorageObject, type User };
