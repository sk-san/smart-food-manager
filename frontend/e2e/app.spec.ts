import { test, expect, Page } from "@playwright/test";

// End-to-end tests for the food manager UI. The Vite dev server is real;
// backend endpoints are stubbed with page.route so the tests are
// deterministic and never call the Go API or Gemini.

const ANALYZE_PATH = "**/api/v1/nutrition/analyze";
const TELEMETRY_PATH = "**/api/v1/telemetry/logs";
const LOGIN_PATH = "**/api/v1/auth/login";

// App.tsx seeds isLoggedIn from getToken(), and api/client.ts reads the token
// out of localStorage once at module load. Planting one before any app code
// runs is what puts these tests inside the app instead of on the login screen.
const AUTH_TOKEN = "e2e-token";

// Tab buttons live in the header's <nav>. The avatar (aria-label "Account")
// and the brand (aria-label "Today") sit in the same <header> outside that
// <nav>, so anything scoped to the header alone matches two elements.
function navTab(page: Page, name: string) {
  return page.locator("header nav").getByRole("button", { name });
}

// Header controls that are not tabs. The mobile bottom bar repeats these
// names, so stay scoped to the <header>.
function headerButton(page: Page, name: string) {
  return page.locator("header").getByRole("button", { name });
}

// The active view. The header carries its own "Log Out" alongside the one in
// the account screen, and role names match case-insensitively, so screen
// content has to be addressed through <main>.
function content(page: Page) {
  return page.locator("main");
}

// LoginView refuses to submit until both fields are filled, so signing in is
// always fill-then-click. The credentials are arbitrary: LOGIN_PATH is stubbed.
async function signIn(page: Page) {
  await page.getByLabel("Email").fill("john.doe@example.com");
  await page.getByLabel("Password").fill("correct-horse");
  await page.getByRole("button", { name: "Sign in" }).click();
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript((token) => {
    localStorage.setItem("auth_token", token);
  }, AUTH_TOKEN);

  // Telemetry batches flush on a timer; accept them so the console stays
  // clean, and keep a stub in place before any app code runs.
  await page.route(TELEMETRY_PATH, (route) =>
    route.fulfill({ status: 202, json: { accepted: 0, dropped: 0 } })
  );
  // The companion character polls this endpoint; without a stub it spams
  // proxy errors in the dev-server log (the UI falls back locally either way).
  await page.route("**/api/v1/companion/message", (route) =>
    route.fulfill({ status: 200, json: { message: "(o^▽^o) keep it up!" } })
  );
  // Sign-in posts real credentials to the Go API; stub it so the login tests
  // stay deterministic and need no database.
  await page.route(LOGIN_PATH, (route) =>
    route.fulfill({ status: 200, json: { token: AUTH_TOKEN } })
  );
  await page.goto("/");
});

test.describe("dashboard", () => {
  test("shows an empty ledger and the calorie goal", async ({ page }) => {
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();

    // The log starts empty; the hero card shows a zero total against the goal.
    await expect(page.getByText(/Nothing on the table yet/)).toBeVisible();
    await expect(page.getByText("of 2,200 kcal", { exact: true })).toBeVisible();
    await expect(page.getByText("0", { exact: true })).toBeVisible();
  });
});

test.describe("navigation", () => {
  test("switches between the three views", async ({ page }) => {
    await navTab(page, "Stats").click();
    await expect(page.getByRole("heading", { name: "Statistics" })).toBeVisible();

    await navTab(page, "Account").click();
    await expect(page.getByRole("heading", { name: "Your account" })).toBeVisible();

    await navTab(page, "Today").click();
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
  });

  test("emits telemetry batches to the ingest endpoint", async ({ page }) => {
    // Navigating queues navigation + screen_view events; the logger flushes
    // them to /api/v1/telemetry/logs within FLUSH_INTERVAL_MS (5s).
    const batch = page.waitForRequest(TELEMETRY_PATH, { timeout: 10_000 });
    await navTab(page, "Stats").click();
    const req = await batch;

    const body = req.postDataJSON() as { events: Array<{ "event.name": string }> };
    const names = body.events.map((e) => e["event.name"]);
    expect(names).toContain("navigation");
  });
});

test.describe("log food modal", () => {
  test("adds AI-analyzed items to the daily log", async ({ page }) => {
    // The backend answers with a bare array of analyzed items.
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [
          {
            name: "Avocado Toast",
            calories: 320,
            protein: 9,
            carbs: 30,
            fat: 18,
            sodium: 400,
            calcium: 60,
            iron: 2,
          },
        ],
      })
    );

    await headerButton(page, "Log food").click();
    await expect(page.getByRole("heading", { name: "Log Food" })).toBeVisible();

    // Analyze is disabled until there is input.
    const analyze = page.getByRole("button", { name: /Analyze/ });
    await expect(analyze).toBeDisabled();

    await page.getByPlaceholder(/grilled chicken sandwich/).fill("avocado toast with egg");
    await analyze.click();

    // Review step shows the stubbed item, then confirming adds it.
    await expect(page.getByText("Found Items")).toBeVisible();
    await expect(page.getByText("Avocado Toast")).toBeVisible();
    await page.getByRole("button", { name: "Add to Log" }).click();

    // Modal closes and the dashboard reflects the new entry and total.
    await expect(page.getByRole("heading", { name: "Log Food" })).toBeHidden();
    await expect(page.getByText("Avocado Toast")).toBeVisible();
    await expect(page.getByText("320", { exact: true })).toBeVisible();
  });

  test("falls back to an estimated item when the backend fails", async ({ page }) => {
    // nutritionService deliberately swallows API errors and returns a
    // fallback item named after the input, so the modal stays usable.
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({ status: 502, json: { error: "analysis failed" } })
    );

    await headerButton(page, "Log food").click();
    await page.getByPlaceholder(/grilled chicken sandwich/).fill("mystery stew");
    await page.getByRole("button", { name: /Analyze/ }).click();

    await expect(page.getByText("Found Items")).toBeVisible();
    await expect(page.getByText("mystery stew")).toBeVisible();
  });
});

test.describe("account", () => {
  test("logging out shows the template login page and sign in returns", async ({ page }) => {
    await navTab(page, "Account").click();
    await content(page).getByRole("button", { name: "Log out" }).click();

    // Logging out clears the token, so the sign-in screen replaces the app.
    await expect(page.getByRole("heading", { name: "Welcome back to the table." })).toBeVisible();
    await expect(page.getByLabel("Email")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();

    await signIn(page);
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
  });

  test("guest bypass buttons enter the app with the chosen role", async ({ page }) => {
    await navTab(page, "Account").click();
    await content(page).getByRole("button", { name: "Log out" }).click();

    // Developer bypass: straight to the dashboard, identity reflects the role.
    await page.getByRole("button", { name: "Developer" }).click();
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
    await navTab(page, "Account").click();
    await expect(page.getByText("dev@nutri.local")).toBeVisible();

    // Alpha bypass works the same way.
    await content(page).getByRole("button", { name: "Log out" }).click();
    await page.getByRole("button", { name: "Alpha tester" }).click();
    await navTab(page, "Account").click();
    await expect(page.getByText("alpha@nutri.local")).toBeVisible();

    // A normal sign-in clears the guest role and returns to the demo account.
    await content(page).getByRole("button", { name: "Log out" }).click();
    await signIn(page);
    await navTab(page, "Account").click();
    await expect(page.getByText("john.doe@example.com")).toBeVisible();
  });

  test("saving a new calorie goal updates the dashboard", async ({ page }) => {
    await navTab(page, "Account").click();

    await page.getByRole("spinbutton", { name: "Calories (kcal)" }).fill("2500");
    await page.getByRole("button", { name: "Save goals" }).click();

    await navTab(page, "Today").click();
    await expect(page.getByText("of 2,500 kcal", { exact: true })).toBeVisible();
  });
});
