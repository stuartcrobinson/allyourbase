import { describe, it, expect, vi, beforeEach } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";

function mockFetch(
  status: number,
  body: unknown,
  headers?: Record<string, string>,
): typeof globalThis.fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    headers: new Headers(headers),
    json: () => Promise.resolve(body),
  }) as unknown as typeof globalThis.fetch;
}

describe("AYBClient", () => {
  it("constructs with baseURL", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("strips trailing slash from baseURL", () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090/", { fetch: fetchFn });
    client.records.list("posts");
    expect(fetchFn).toHaveBeenCalledWith(
      "http://localhost:8090/api/collections/posts",
      expect.anything(),
    );
  });

  it("setTokens / clearTokens", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("access", "refresh");
    expect(client.token).toBe("access");
    expect(client.refreshToken).toBe("refresh");
    client.clearTokens();
    expect(client.token).toBeNull();
  });
});

describe("records", () => {
  let fetchFn: ReturnType<typeof mockFetch>;
  let client: AYBClient;

  beforeEach(() => {
    fetchFn = mockFetch(200, { items: [{ id: "1" }], page: 1, perPage: 20, totalItems: 1, totalPages: 1 });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
  });

  it("list sends correct URL", async () => {
    await client.records.list("posts", { page: 2, sort: "-created_at", filter: "active=true" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/collections/posts?");
    expect(url).toContain("page=2");
    expect(url).toContain("sort=-created_at");
    expect(url).toContain("filter=active%3Dtrue");
  });

  it("list with no params", async () => {
    await client.records.list("posts");
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("http://localhost:8090/api/collections/posts");
  });

  it("get sends correct URL", async () => {
    fetchFn = mockFetch(200, { id: "42", title: "hello" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.get("posts", "42", { expand: "author" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/collections/posts/42");
    expect(url).toContain("expand=author");
  });

  it("create sends POST with body", async () => {
    fetchFn = mockFetch(201, { id: "new", title: "test" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.create("posts", { title: "test" });
    expect(result).toEqual({ id: "new", title: "test" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ title: "test" });
  });

  it("update sends PATCH", async () => {
    fetchFn = mockFetch(200, { id: "42", title: "updated" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.update("posts", "42", { title: "updated" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/collections/posts/42");
    expect(call[1].method).toBe("PATCH");
  });

  it("delete sends DELETE", async () => {
    fetchFn = mockFetch(204, undefined);
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.delete("posts", "42");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/collections/posts/42");
    expect(call[1].method).toBe("DELETE");
  });
});

describe("auth", () => {
  it("login stores tokens", async () => {
    const fetchFn = mockFetch(200, { token: "tok", refreshToken: "ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const res = await client.auth.login("a@b.com", "pass");
    expect(res.token).toBe("tok");
    expect(client.token).toBe("tok");
    expect(client.refreshToken).toBe("ref");
  });

  it("register stores tokens", async () => {
    const fetchFn = mockFetch(201, { token: "tok", refreshToken: "ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.register("a@b.com", "pass");
    expect(client.token).toBe("tok");
  });

  it("logout clears tokens", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.logout();
    expect(client.token).toBeNull();
  });

  it("sends auth header when token is set", async () => {
    const fetchFn = mockFetch(200, { id: "1", email: "a@b.com" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("my-token", "my-refresh");
    await client.auth.me();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer my-token");
  });

  it("requestPasswordReset sends POST", async () => {
    const fetchFn = mockFetch(200, { message: "ok" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.requestPasswordReset("a@b.com");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/password-reset");
    expect(call[1].method).toBe("POST");
  });
});

describe("storage", () => {
  it("downloadURL builds correct URL with bucket and name", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.storage.downloadURL("avatars", "photo.jpg")).toBe(
      "http://localhost:8090/api/storage/avatars/photo.jpg",
    );
  });

  it("upload sends POST to /api/storage/{bucket}", async () => {
    const fetchFn = mockFetch(201, { id: "1", bucket: "avatars", name: "photo.jpg" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const file = new Blob(["test"], { type: "image/jpeg" });
    await client.storage.upload("avatars", file, "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars");
    expect(call[1].method).toBe("POST");
  });

  it("delete sends DELETE to /api/storage/{bucket}/{name}", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.delete("avatars", "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars/photo.jpg");
    expect(call[1].method).toBe("DELETE");
  });

  it("list sends GET to /api/storage/{bucket}", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { prefix: "user_", limit: 10 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/storage/avatars?");
    expect(url).toContain("prefix=user_");
    expect(url).toContain("limit=10");
  });

  it("getSignedURL sends POST to /api/storage/{bucket}/{name}/sign", async () => {
    const fetchFn = mockFetch(200, { url: "/api/storage/avatars/photo.jpg?exp=123&sig=abc" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.getSignedURL("avatars", "photo.jpg", 7200);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars/photo.jpg/sign");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ expiresIn: 7200 });
  });
});

describe("error handling", () => {
  it("throws AYBError on non-2xx", async () => {
    const fetchFn = mockFetch(404, { message: "collection not found: missing" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.records.list("missing")).rejects.toThrow(AYBError);
    await expect(client.records.list("missing")).rejects.toThrow("collection not found: missing");
  });

  it("AYBError has status", async () => {
    const fetchFn = mockFetch(401, { message: "unauthorized" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    try {
      await client.auth.me();
    } catch (e) {
      expect(e).toBeInstanceOf(AYBError);
      expect((e as AYBError).status).toBe(401);
    }
  });
});
