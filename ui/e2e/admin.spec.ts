import { test, expect } from "@playwright/test";

// These tests run against a live ayb server (localhost:8090).
// Start with: go build -o ayb ./cmd/ayb && ./ayb start
//
// IMPORTANT: No sleep() or waitForTimeout() — Playwright locators auto-wait.

test.describe("Admin Dashboard (no auth)", () => {
  test("loads the admin page", async ({ page }) => {
    await page.goto("/admin/");

    // The app should render — either the dashboard or "Loading..."
    // then settle into the dashboard (or "No tables found" for empty DB).
    await expect(
      page.getByText("AYB Admin").first(),
    ).toBeVisible();
  });

  test("shows sidebar with AYB branding", async ({ page }) => {
    await page.goto("/admin/");

    await expect(page.locator("aside")).toBeVisible();
    await expect(
      page.locator("aside").getByText("AYB Admin"),
    ).toBeVisible();
  });

  test("shows empty state when no user tables", async ({ page }) => {
    await page.goto("/admin/");

    // With an empty database, expect either "No tables found" in sidebar
    // or "Select a table" in main content area.
    const noTables = page.getByText("No tables found");
    const selectTable = page.getByText("Select a table");
    await expect(noTables.or(selectTable).first()).toBeVisible();
  });

  test("SPA fallback serves app on deep routes", async ({ page }) => {
    await page.goto("/admin/some/deep/route");

    // Should still load the SPA, not a 404 page.
    await expect(
      page.getByText("AYB Admin").first(),
    ).toBeVisible();
  });

  test("API status endpoint is accessible", async ({ request }) => {
    const res = await request.get("/api/admin/status");
    expect(res.status()).toBe(200);

    const body = await res.json();
    expect(body).toHaveProperty("auth");
    expect(typeof body.auth).toBe("boolean");
  });

  test("schema endpoint returns valid response", async ({ request }) => {
    const res = await request.get("/api/schema");
    expect(res.status()).toBe(200);

    const body = await res.json();
    expect(body).toHaveProperty("tables");
    expect(body).toHaveProperty("schemas");
    expect(body).toHaveProperty("builtAt");
  });
});

test.describe("Admin Dashboard (with auth)", () => {
  // These tests need the server started with admin.password set.
  // Skip if auth is not configured.

  test("shows login form when auth required", async ({ page, request }) => {
    const status = await request.get("/api/admin/status");
    const { auth } = await status.json();
    test.skip(!auth, "admin.password not configured");

    await page.goto("/admin/");
    await expect(page.getByText("Enter the admin password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("rejects wrong password", async ({ page, request }) => {
    const status = await request.get("/api/admin/status");
    const { auth } = await status.json();
    test.skip(!auth, "admin.password not configured");

    await page.goto("/admin/");
    await page.getByLabel("Password").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign in" }).click();

    await expect(page.getByText("invalid password")).toBeVisible();
  });
});
