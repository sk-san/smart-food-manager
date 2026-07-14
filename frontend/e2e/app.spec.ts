import { test, expect, Page } from "@playwright/test";

// End-to-end tests for the food manager UI. The Vite dev server is real;
// backend endpoints are stubbed with page.route so the tests are
// deterministic and never call the Go API or Gemini.

// The two demo entries seeded in App.tsx: 350 + 450 kcal.
const SEEDED_CALORIES = 800;

const ANALYZE_PATH = "**/api/v1/nutrition/analyze";
const TELEMETRY_PATH = "**/api/v1/telemetry/logs";

// The desktop navigation rail (the mobile bottom bar duplicates the same
// button names, so scope clicks to the <aside>).
function navRail(page: Page) {
  return page.locator("aside");
}

test.beforeEach(async ({ page }) => {
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
  await page.goto("/");
});

test.describe("dashboard", () => {
  test("shows the seeded food log and computed totals", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();

    // Seeded entries appear in the log.
    await expect(page.getByText("Oatmeal & Blueberries")).toBeVisible();
    await expect(page.getByText("Grilled Chicken Salad")).toBeVisible();

    // The calories hero card sums the entries and shows the goal.
    await expect(page.getByText(String(SEEDED_CALORIES), { exact: true })).toBeVisible();
    await expect(page.getByText("Daily Limit: 2200")).toBeVisible();
  });
});

test.describe("navigation", () => {
  test("switches between the three views", async ({ page }) => {
    await navRail(page).getByRole("button", { name: "Stats" }).click();
    await expect(page.getByRole("heading", { name: "Statistics" })).toBeVisible();

    await navRail(page).getByRole("button", { name: "Prefs" }).click();
    await expect(page.getByRole("heading", { name: "Preferences" })).toBeVisible();

    await navRail(page).getByRole("button", { name: "Dash" }).click();
    await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();
  });

  test("emits telemetry batches to the ingest endpoint", async ({ page }) => {
    // Navigating queues navigation + screen_view events; the logger flushes
    // them to /api/v1/telemetry/logs within FLUSH_INTERVAL_MS (5s).
    const batch = page.waitForRequest(TELEMETRY_PATH, { timeout: 10_000 });
    await navRail(page).getByRole("button", { name: "Stats" }).click();
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

    await page.getByRole("button", { name: "Log food" }).click();
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
    await expect(
      page.getByText(String(SEEDED_CALORIES + 320), { exact: true })
    ).toBeVisible();
  });

  test("falls back to an estimated item when the backend fails", async ({ page }) => {
    // nutritionService deliberately swallows API errors and returns a
    // fallback item named after the input, so the modal stays usable.
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({ status: 502, json: { error: "analysis failed" } })
    );

    await page.getByRole("button", { name: "Log food" }).click();
    await page.getByPlaceholder(/grilled chicken sandwich/).fill("mystery stew");
    await page.getByRole("button", { name: /Analyze/ }).click();

    await expect(page.getByText("Found Items")).toBeVisible();
    await expect(page.getByText("mystery stew")).toBeVisible();
  });
});

test.describe("preferences", () => {
  test("saving a new calorie goal updates the dashboard", async ({ page }) => {
    await navRail(page).getByRole("button", { name: "Prefs" }).click();

    const caloriesInput = page
      .locator("div", { has: page.getByText("Daily Calories", { exact: true }) })
      .locator("input")
      .first();
    await caloriesInput.fill("2500");
    await page.getByRole("button", { name: "Save" }).click();

    await navRail(page).getByRole("button", { name: "Dash" }).click();
    await expect(page.getByText("Daily Limit: 2500")).toBeVisible();
  });
});
