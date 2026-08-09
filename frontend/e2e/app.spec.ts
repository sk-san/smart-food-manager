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

// Both the desktop header nav and the mobile tab bar are labelled "Primary",
// but only one is displayed at a time — the other is `display: none` and so is
// out of the accessibility tree. Going through the navigation role therefore
// resolves to whichever chrome the current viewport is showing.
function tab(page: Page, name: string) {
  return page.getByRole("navigation").getByRole("button", { name, exact: true });
}

const PHONE = { width: 390, height: 844 };

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

test.describe("navigation semantics", () => {
  test("marks the current tab and labels the primary nav", async ({ page }) => {
    const nav = page.getByRole("navigation", { name: "Primary" });
    await expect(nav).toHaveCount(1);

    await expect(tab(page, "Today")).toHaveAttribute("aria-current", "page");
    await expect(tab(page, "Stats")).not.toHaveAttribute("aria-current", "page");

    await tab(page, "Stats").click();
    await expect(tab(page, "Stats")).toHaveAttribute("aria-current", "page");
    await expect(tab(page, "Today")).not.toHaveAttribute("aria-current", "page");
  });

  // Only the phone can switch tabs from halfway down a screen: its tab bar is
  // fixed to the bottom, where the desktop header scrolls away with the page.
  test.describe("on a phone", () => {
    test.use({ viewport: PHONE });

    test("opens a new tab at the top and restores the one you left", async ({ page }) => {
      await tab(page, "Account").click();
      await expect(page.getByRole("heading", { name: "Your account" })).toBeVisible();

      await page.mouse.wheel(0, 500);
      await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(100);
      const left = await page.evaluate(() => window.scrollY);

      // A tab you have not visited starts at its own beginning, not at the
      // offset the previous screen happened to be scrolled to.
      await tab(page, "Today").click();
      expect(await page.evaluate(() => window.scrollY)).toBe(0);

      // Coming back returns you to where you were reading.
      await tab(page, "Account").click();
      expect(await page.evaluate(() => window.scrollY)).toBe(left);
    });
  });
});

test.describe("log food dialog", () => {
  test("is a modal dialog that Escape closes, returning focus to its trigger", async ({ page }) => {
    const trigger = headerButton(page, "Log food");
    await trigger.click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toHaveAttribute("aria-modal", "true");
    // Named by its own heading rather than a hand-written label.
    await expect(dialog).toHaveAccessibleName("Log Food");

    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(trigger).toBeFocused();
  });

  test("keeps Tab inside the dialog", async ({ page }) => {
    await headerButton(page, "Log food").click();
    await expect(page.getByRole("dialog")).toBeVisible();

    // Twenty tabs is more than the dialog holds, so an untrapped focus ring
    // would have escaped into the page behind by now.
    for (let i = 0; i < 20; i++) await page.keyboard.press("Tab");
    const inside = await page.evaluate(
      () => !!document.activeElement?.closest('[role="dialog"]')
    );
    expect(inside).toBe(true);
  });

  test("labels its inputs and makes the upload region a real control", async ({ page }) => {
    await headerButton(page, "Log food").click();

    // Text mode: the textarea has a visible, associated label.
    await expect(page.getByLabel("What did you eat?")).toBeVisible();

    // Image mode: uploading is a button, so it is in the tab order and
    // responds to Enter and Space — it used to be a bare clickable <div>.
    await page.getByRole("button", { name: "Image" }).click();
    const upload = page.getByRole("button", { name: "Food photo" });
    await expect(upload).toBeVisible();
    await upload.focus();
    await expect(upload).toBeFocused();
  });

  test("announces progress instead of alerting on failure", async ({ page }) => {
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({ status: 200, json: [] })
    );

    await headerButton(page, "Log food").click();
    await page.getByLabel("What did you eat?").fill("something unrecognisable");

    // A polite live region carries the in-flight state.
    await expect(page.locator('[role="status"]')).toBeAttached();

    await page.getByRole("button", { name: /Analyze/ }).click();

    // An empty result reports itself in the dialog rather than in window.alert,
    // which no screen reader and no keyboard user could deal with.
    await expect(page.getByRole("alert")).toContainText(/No food was recognised/);
    await expect(page.getByRole("dialog")).toBeVisible();
  });
});

test.describe("dashboard layout preference", () => {
  test("is chosen in Account and applies to Today", async ({ page }) => {
    // Today shows one dashboard; the alternatives are personalization, so the
    // switcher lives in Account rather than on the dashboard itself.
    await expect(content(page).getByRole("radio", { name: /Plate/ })).toHaveCount(0);

    await tab(page, "Account").click();
    await page.getByRole("radio", { name: /Almanac/ }).check();

    await tab(page, "Today").click();
    // The Almanac is offered at every width now, desktop included.
    await expect(page.getByRole("button", { name: /Log (breakfast|lunch|supper)/ })).toBeVisible();

    await tab(page, "Account").click();
    await page.getByRole("radio", { name: /Ledger/ }).check();
    await tab(page, "Today").click();
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
  });
});

test.describe("false affordances", () => {
  test("controls with nothing behind them say so", async ({ page }) => {
    await tab(page, "Pantry").click();

    const addItem = page.getByRole("button", { name: "Add item" });
    await expect(addItem).toHaveAttribute("aria-disabled", "true");
    // The reason is on the page, not only in a tooltip.
    await expect(addItem).toHaveAccessibleDescription(/seed data/);

    // Reminders and sound effects no longer render as switches you can flip.
    await tab(page, "Account").click();
    await expect(page.getByRole("switch", { name: "Dark mode" })).toBeVisible();
    await expect(page.getByRole("switch", { name: /Reminders/ })).toHaveCount(0);
    await expect(page.getByRole("switch", { name: /Sound effects/ })).toHaveCount(0);

    // "View all" pointed at a list that was already fully shown.
    await tab(page, "Today").click();
    await expect(page.getByRole("button", { name: "View all" })).toHaveCount(0);
  });
});

test.describe("phone layout", () => {
  test.use({ viewport: PHONE });

  test("shows the larder as an expiry-sorted list with no sideways scrolling", async ({ page }) => {
    await tab(page, "Pantry").click();

    // The timeline canvas is a desktop-only option; the phone gets the list.
    await expect(page.getByRole("button", { name: "Timeline" })).toHaveCount(0);

    const items = content(page).locator("li");
    await expect(items.first()).toContainText("Baby spinach");
    // Everything the timeline hid behind a hover tooltip is now written out.
    await expect(items.first()).toContainText("1 day left");

    const overflows = await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth
    );
    expect(overflows).toBe(false);
  });

  test("gives the icon-only log out an accessible name and a 44px target", async ({ page }) => {
    const logout = page.getByRole("button", { name: "Log out" }).first();
    await expect(logout).toBeVisible();

    const box = await logout.boundingBox();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
  });

  test("folds the companion away by default and remembers being reopened", async ({ page }) => {
    // Nothing narrower than a wide desktop has a gutter to spare, so Nutri
    // starts as a badge rather than parked on top of a card.
    const reopen = page.getByRole("button", { name: /Show Nutri/ });
    await expect(reopen).toBeVisible();

    await reopen.click();
    const cat = page.getByRole("button", { name: /nutrition companion/ });
    await expect(cat).toBeVisible();

    await page.reload();
    await expect(page.getByRole("button", { name: /nutrition companion/ })).toBeVisible();

    await page.getByRole("button", { name: "Hide Nutri" }).click();
    await expect(page.getByRole("button", { name: /Show Nutri/ })).toBeVisible();
  });

  test("hides the companion while the log food dialog is open", async ({ page }) => {
    await expect(page.getByRole("button", { name: /Show Nutri/ })).toBeVisible();

    await page.getByRole("button", { name: "Log food" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(page.getByRole("button", { name: /Nutri/ })).toHaveCount(0);

    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: /Show Nutri/ })).toBeVisible();
  });
});
