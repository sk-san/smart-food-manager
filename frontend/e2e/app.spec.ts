import { test, expect, Page } from "@playwright/test";

// End-to-end tests for the food manager UI. The Vite dev server is real;
// backend endpoints are stubbed with page.route so the tests are
// deterministic and never call the Go API or Gemini.

const ANALYZE_PATH = "**/api/v1/nutrition/analyze";
const QUOTA_PATH = "**/api/v1/nutrition/quota";
const TELEMETRY_PATH = "**/api/v1/telemetry/logs";
const LOGIN_PATH = "**/api/v1/auth/login";
const ME_PATH = "**/api/v1/me";
const MEALS_PATH = "**/api/v1/meals";
const GOALS_PATH = "**/api/v1/goals";
const INVENTORY_PATH = "**/api/v1/inventory";
const INVENTORY_SCAN_PATH = "**/api/v1/inventory/scans";
const INVENTORY_CONSUME_PATH = /\/api\/v1\/inventory\/[^/]+\/consume$/;
const WASTE_PATH = "**/api/v1/waste-events";

// api/client.ts reads the token out of localStorage once at module load. App
// verifies that stored token through /api/v1/me before rendering private UI.
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

const round = (value: number) => Number(value.toFixed(6));

const scaleNutrients = (per100g: Record<string, number>, quantityGrams: number) => {
  const scale = quantityGrams / 100;
  return {
    calories: round((per100g.calories ?? 0) * scale),
    protein: round((per100g.protein ?? 0) * scale),
    carbs: round((per100g.carbs ?? 0) * scale),
    fat: round((per100g.fat ?? 0) * scale),
    sodium: round((per100g.sodium ?? 0) * scale),
    calcium: round((per100g.calcium ?? 0) * scale),
    iron: round((per100g.iron ?? 0) * scale),
  };
};

const nutrientsPer100g = (totals: Record<string, number>, quantityGrams: number) => {
  const scale = 100 / quantityGrams;
  return {
    calories: round((totals.calories ?? 0) * scale),
    protein: round((totals.protein ?? 0) * scale),
    carbs: round((totals.carbs ?? 0) * scale),
    fat: round((totals.fat ?? 0) * scale),
    sodium: round((totals.sodium ?? 0) * scale),
    calcium: round((totals.calcium ?? 0) * scale),
    iron: round((totals.iron ?? 0) * scale),
  };
};

// Deterministic fixture factors: 2 kg CO2e, 2,000 L virtual water, and 0.02
// urban tree-years per kilogram of food. The UI only aggregates response
// fields; the production backend owns the real category-based factors.
const impactForQuantity = (quantityGrams: number) => ({
  impact_kg_co2e: round(quantityGrams * 0.002),
  virtual_water_l: round(quantityGrams * 2),
  tree_equivalents: round(quantityGrams * 0.00002),
});

// LoginView refuses to submit until both fields are filled, so signing in is
// always fill-then-click. The credentials are arbitrary: LOGIN_PATH is stubbed.
async function signIn(page: Page) {
  await page.getByLabel("Email").fill("john.doe@example.com");
  await page.getByLabel("Password").fill("correct-horse");
  await page.getByRole("button", { name: "Sign in" }).click();
}

// The suite starts signed in (beforeEach seeds a token), so reaching the guest
// identity means leaving the account first.
async function enterAsGuest(page: Page) {
  await navTab(page, "Account").click();
  await content(page).getByRole("button", { name: "Log out" }).click();
  await page.getByRole("button", { name: "Continue as guest" }).click();
  await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
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
  // The scanner asks how many AI analyses a guest has left. Signed-in callers
  // are uncapped, which is what the app tests below assume; the guest quota
  // tests override this route with a real allowance.
  await page.route(QUOTA_PATH, (route) =>
    route.fulfill({ status: 200, json: { unlimited: true, limit: 0, used: 0, remaining: 0 } })
  );
  // Sign-in posts real credentials to the Go API; stub it so the login tests
  // stay deterministic and need no database.
  await page.route(LOGIN_PATH, (route) =>
    route.fulfill({ status: 200, json: { token: AUTH_TOKEN } })
  );
  await page.route(ME_PATH, (route) =>
    route.fulfill({ status: 200, json: { user_id: "user-e2e", roles: ["user"] } })
  );
  const savedMeals: Record<string, any>[] = [];
  let nextMealId = 1;
  let nextInventoryId = 1;
  let nextWasteId = 1;
  await page.route(MEALS_PATH, (route) => {
    if (route.request().method() === "GET") return route.fulfill({ status: 200, json: savedMeals });
    const meal = route.request().postDataJSON();
    const saved = { ...meal, id: `meal-e2e-${nextMealId++}` };
    savedMeals.push(saved);
    return route.fulfill({ status: 201, json: saved });
  });
  let savedGoals = { calories: 2200, protein: 150, carbs: 250, fat: 70, sodium: 2300, calcium: 1000, iron: 18 };
  await page.route(GOALS_PATH, (route) => {
    if (route.request().method() === "PUT") {
      savedGoals = route.request().postDataJSON();
      return route.fulfill({ status: 200, json: savedGoals });
    }
    return route.fulfill({ status: 200, json: savedGoals });
  });
  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  const expiry = `${tomorrow.getFullYear()}-${String(tomorrow.getMonth() + 1).padStart(2, "0")}-${String(tomorrow.getDate()).padStart(2, "0")}`;
  const inventoryItems: Record<string, any>[] = [{
    id: "inventory-spinach",
    food_id: "food-spinach",
    name: "Baby spinach",
    quantity_purchased: 200,
    quantity_consumed: 0,
    quantity_wasted: 0,
    best_before_date: expiry,
    date_label: "best_before",
    storage: "fridge",
    package: "unopened",
    source_type: "ingredient",
    category: "vegetables",
    expiry_is_estimated: false,
    nutrition_per_100g: {
      calories: 23,
      protein: 2.9,
      carbs: 3.6,
      fat: 0.4,
      sodium: 79,
      calcium: 99,
      iron: 2.7,
    },
    is_wasted: false,
    is_resolved: false,
    consumed_pct: 0,
    wasted_pct: 0,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }];
  const wasteEvents: Record<string, any>[] = [];

  const saveWasteEvent = (
    item: Record<string, any>,
    quantityGrams: number,
    input: Record<string, any>,
  ) => {
    const saved = {
      ...input,
      id: `waste-e2e-${nextWasteId++}`,
      inventory_item_id: item.id,
      food_id: item.food_id,
      food_name: item.name,
      quantity_g: quantityGrams,
      wasted_at: input.wasted_at ?? new Date().toISOString(),
      reason: input.reason ?? "other",
      date_label: input.date_label ?? item.date_label ?? "unknown",
      date_status: input.date_status ?? "unknown",
      package: input.package ?? item.package ?? "unknown",
      spoilage: input.spoilage ?? "unknown",
      classification: input.classification ?? "expiry_unrelated",
      category: item.category ?? "other",
      created_at: new Date().toISOString(),
      ...impactForQuantity(quantityGrams),
    };
    wasteEvents.unshift(saved);
    return saved;
  };

  await page.route(INVENTORY_PATH, (route) => {
    if (route.request().method() === "POST") {
      const item = route.request().postDataJSON();
      const saved = {
        ...item,
        id: `inventory-created-${nextInventoryId}`,
        food_id: `food-created-${nextInventoryId++}`,
        quantity_wasted: 0,
        is_wasted: false,
        is_resolved: false,
        consumed_pct: 0,
        wasted_pct: 0,
        date_label: item.date_label ?? "unknown",
        package: item.package ?? "unopened",
        source_type: item.source_type ?? "ingredient",
        category: item.category ?? "other",
        expiry_is_estimated: item.expiry_is_estimated ?? false,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      inventoryItems.push(saved);
      return route.fulfill({ status: 201, json: saved });
    }
    return route.fulfill({ status: 200, json: inventoryItems });
  });

  await page.route(INVENTORY_SCAN_PATH, (route) => {
    if (route.request().method() !== "POST") return route.fulfill({ status: 405, body: "method not allowed" });
    const input = route.request().postDataJSON() as Record<string, any>;
    const now = new Date().toISOString();
    const inventoryId = `inventory-scan-${nextInventoryId}`;
    const foodId = `food-scan-${nextInventoryId++}`;
    const provisionalMeal = input.source_type === "ingredient"
      ? undefined
      : {
          id: `meal-scan-${nextMealId++}`,
          name: input.name,
          consumed_at: input.consumed_at ?? now,
          ...input.nutrients,
        };
    if (provisionalMeal) savedMeals.push(provisionalMeal);

    const savedInventory: Record<string, any> = {
      id: inventoryId,
      food_id: foodId,
      name: input.name,
      quantity_purchased: input.quantity_g,
      quantity_consumed: 0,
      quantity_wasted: 0,
      ...(input.date_label === "best_before"
        ? { best_before_date: input.expiry_date }
        : { use_by_date: input.expiry_date }),
      date_label: input.date_label,
      storage: input.storage,
      package: input.package,
      source_type: input.source_type,
      category: input.category ?? "other",
      expiry_is_estimated: input.expiry_is_estimated,
      nutrition_per_100g: nutrientsPer100g(input.nutrients, input.quantity_g),
      provisional_meal_id: provisionalMeal?.id,
      is_wasted: false,
      is_resolved: false,
      consumed_pct: 0,
      wasted_pct: 0,
      created_at: now,
      updated_at: now,
    };
    inventoryItems.push(savedInventory);
    return route.fulfill({
      status: 201,
      json: { inventory: savedInventory, ...(provisionalMeal ? { meal: provisionalMeal } : {}) },
    });
  });

  await page.route(INVENTORY_CONSUME_PATH, (route) => {
    if (route.request().method() !== "POST") return route.fulfill({ status: 405, body: "method not allowed" });
    const path = new URL(route.request().url()).pathname.split("/");
    const inventoryId = path[path.length - 2];
    const item = inventoryItems.find((candidate) => candidate.id === inventoryId);
    if (!item) return route.fulfill({ status: 404, json: { error: "pantry item not found" } });

    const input = route.request().postDataJSON() as Record<string, any>;
    const quantity = Number(input.quantity_g);
    const purchased = Number(item.quantity_purchased);
    const consumedBefore = Number(item.quantity_consumed ?? 0);
    const wastedBefore = Number(item.quantity_wasted ?? 0);
    const remainingBefore = purchased - consumedBefore - wastedBefore;
    const discarded = input.discard_remaining ? Math.max(0, remainingBefore - quantity) : 0;
    const newConsumed = consumedBefore + quantity;
    const newWasted = wastedBefore + discarded;
    const now = new Date().toISOString();
    let meal: Record<string, any> | undefined;
    let deletedMealId: string | undefined;

    if (item.provisional_meal_id) {
      const provisionalId = item.provisional_meal_id;
      const existingIndex = savedMeals.findIndex((candidate) => candidate.id === provisionalId);
      if (newConsumed === 0) {
        if (existingIndex >= 0) savedMeals.splice(existingIndex, 1);
        deletedMealId = provisionalId;
      } else {
        meal = {
          id: provisionalId,
          name: item.name,
          consumed_at: existingIndex >= 0 ? savedMeals[existingIndex].consumed_at : now,
          ...scaleNutrients(item.nutrition_per_100g, newConsumed),
        };
        if (existingIndex >= 0) savedMeals[existingIndex] = meal;
        else savedMeals.push(meal);
      }
      delete item.provisional_meal_id;
    } else if (quantity > 0) {
      meal = {
        id: `meal-consume-${nextMealId++}`,
        name: item.name,
        consumed_at: now,
        ...scaleNutrients(item.nutrition_per_100g, quantity),
      };
      savedMeals.push(meal);
    }

    item.quantity_consumed = newConsumed;
    item.quantity_wasted = newWasted;
    item.is_wasted = newWasted > 0;
    item.is_resolved = newConsumed + newWasted >= purchased;
    item.consumed_pct = purchased > 0 ? newConsumed / purchased * 100 : 0;
    item.wasted_pct = purchased > 0 ? newWasted / purchased * 100 : 0;
    item.updated_at = now;

    const wasteEvent = discarded > 0
      ? saveWasteEvent(item, discarded, {
          reason: input.waste_reason,
          date_label: item.date_label,
          package: item.package,
        })
      : undefined;
    return route.fulfill({
      status: 200,
      json: {
        inventory: item,
        ...(meal ? { meal } : {}),
        ...(deletedMealId ? { deleted_meal_id: deletedMealId } : {}),
        ...(wasteEvent ? { waste_event: wasteEvent } : {}),
      },
    });
  });

  await page.route(WASTE_PATH, (route) => {
    if (route.request().method() === "POST") {
      const event = route.request().postDataJSON();
      const item = inventoryItems.find((candidate) => candidate.id === event.inventory_item_id)!;
      item.quantity_wasted += event.quantity_g;
      item.is_wasted = true;
      item.is_resolved = item.quantity_consumed + item.quantity_wasted >= item.quantity_purchased;
      item.wasted_pct = item.quantity_purchased > 0
        ? item.quantity_wasted / item.quantity_purchased * 100
        : 0;
      item.updated_at = new Date().toISOString();
      const saved = saveWasteEvent(item, event.quantity_g, event);
      return route.fulfill({ status: 201, json: saved });
    }
    return route.fulfill({ status: 200, json: wasteEvents });
  });
  await page.goto("/");
});

test.describe("session validation", () => {
  test("clears an invalid persisted token and returns to sign in", async ({ page }) => {
    await page.unroute(ME_PATH);
    await page.route(ME_PATH, (route) =>
      route.fulfill({ status: 401, body: "invalid token\n", contentType: "text/plain" })
    );

    await page.reload();

    await expect(page.getByRole("heading", { name: "Welcome back to the table." })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem("auth_token"))).toBeNull();
  });

  test("preserves the token and retries after a temporary verification failure", async ({ page }) => {
    let backendAvailable = false;
    await page.unroute(ME_PATH);
    await page.route(ME_PATH, (route) =>
      backendAvailable
        ? route.fulfill({ status: 200, json: { user_id: "user-e2e", roles: ["user"] } })
        : route.fulfill({ status: 503, json: { error: "temporarily unavailable" } })
    );

    await page.reload();

    await expect(page.getByRole("heading", { name: "We couldn't verify your session" })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem("auth_token"))).toBe(AUTH_TOKEN);

    backendAvailable = true;
    await page.getByRole("button", { name: "Retry" }).click();
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
  });
});

test.describe("dashboard", () => {
  test("shows an empty ledger and the calorie goal", async ({ page }) => {
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();

    // The log starts empty; the hero card shows a zero total against the goal.
    await expect(page.getByText(/Nothing on the table yet/)).toBeVisible();
    await expect(page.getByText("of 2,200 kcal", { exact: true }).filter({ visible: true })).toBeVisible();
    await expect(page.getByText("0", { exact: true }).filter({ visible: true })).toBeVisible();
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

    await page.getByPlaceholder(/avocado toast/).fill("avocado toast with egg");
    await analyze.click();

    // Review step shows the stubbed item, then confirming adds it.
    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeVisible();
    await expect(page.getByLabel("Food name")).toHaveValue("Avocado Toast");
    await page.getByRole("button", { name: "Add 1 item to today" }).click();

    // Modal closes and the dashboard reflects the new entry and total.
    await expect(page.getByRole("heading", { name: "Log Food" })).toBeHidden();
    await expect(page.getByText("Avocado Toast").filter({ visible: true })).toBeVisible();
    await expect(page.getByText("320", { exact: true }).filter({ visible: true })).toBeVisible();

    // A refresh rehydrates the log from the authenticated meals endpoint.
    await page.reload();
    await expect(page.getByText("Avocado Toast").filter({ visible: true })).toBeVisible();
    await expect(page.getByText("320", { exact: true }).filter({ visible: true })).toBeVisible();
  });

  test("falls back to an estimated item when the backend fails", async ({ page }) => {
    // nutritionService deliberately swallows API errors and returns a
    // fallback item named after the input, so the modal stays usable.
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({ status: 502, json: { error: "analysis failed" } })
    );

    await headerButton(page, "Log food").click();
    await page.getByPlaceholder(/avocado toast/).fill("mystery stew");
    await page.getByRole("button", { name: /Analyze/ }).click();

    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeVisible();
    await expect(page.getByLabel("Food name")).toHaveValue("mystery stew");
  });
});

// AI analysis costs a Gemini call, so visitors without an account get a small
// daily allowance. The backend is the enforcer; these cover what the guest is
// told about it and that the UI never spends a run it does not have.
test.describe("guest AI allowance", () => {
  const RESET_AT = "2026-08-18T00:00:00Z";

  const stubQuota = (page: Page, remaining: number, limit = 3) =>
    page.route(QUOTA_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: { unlimited: false, limit, used: limit - remaining, remaining, resetAt: RESET_AT },
      })
    );

  const analyzedItem = {
    name: "Guest oatmeal",
    calories: 210,
    protein: 7,
    carbs: 36,
    fat: 4,
    sodium: 120,
    calcium: 80,
    iron: 2,
    scanType: "food",
    quantityGrams: 200,
    category: "grains",
    estimatedExpiryDays: 2,
  };

  test("shows the remaining scans and counts one down per analysis", async ({ page }) => {
    await stubQuota(page, 3);
    await page.route(ANALYZE_PATH, (route) => route.fulfill({ status: 200, json: [analyzedItem] }));
    await enterAsGuest(page);

    await headerButton(page, "Log food").click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Guest mode · 3 of 3 AI scans left today")).toBeVisible();

    await page.getByPlaceholder(/avocado toast/).fill("a bowl of oatmeal");
    await page.getByRole("button", { name: /Analyze/ }).click();
    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeVisible();

    // Back on the scanner the spent run is reflected without re-asking the
    // backend, which would report the same allowance the modal already knows.
    await dialog.getByRole("button", { name: "Back to scanner" }).click();
    await expect(dialog.getByText("Guest mode · 2 of 3 AI scans left today")).toBeVisible();
  });

  test("blocks the analysis once the day's scans are gone", async ({ page }) => {
    await stubQuota(page, 0);
    let analyzeCalls = 0;
    await page.route(ANALYZE_PATH, (route) => {
      analyzeCalls += 1;
      return route.fulfill({ status: 200, json: [analyzedItem] });
    });
    await enterAsGuest(page);

    await headerButton(page, "Log food").click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Today’s 3 guest AI scans are used up.")).toBeVisible();

    // The analyze action is gone rather than disabled: there is nothing it
    // could do until the allowance returns.
    await page.getByPlaceholder(/avocado toast/).fill("a bowl of oatmeal");
    await expect(dialog.getByRole("button", { name: /Analyze|Use this photo/ })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeHidden();
    expect(analyzeCalls).toBe(0);
  });

  test("reports a rejected analysis instead of passing off a local estimate", async ({ page }) => {
    // The modal's count is stale — another tab spent the last run — so the
    // rejection arrives from the backend rather than being anticipated.
    await stubQuota(page, 1);
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 429,
        json: {
          error: "guest AI analyses are limited to 3 per day; sign in to continue",
          code: "guest_ai_daily_limit",
          limit: 3,
          remaining: 0,
          resetAt: RESET_AT,
        },
      })
    );
    await enterAsGuest(page);

    await headerButton(page, "Log food").click();
    await page.getByPlaceholder(/avocado toast/).fill("a bowl of oatmeal");
    await page.getByRole("button", { name: /Analyze/ }).click();

    // A text failure normally yields a local estimate; a spent allowance must
    // not, or the guest would think the analysis ran.
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("alert")).toContainText("guest AI scans are used up");
    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeHidden();
    // The rejection also updates the scanner, which now offers no analysis.
    await expect(dialog.getByText("Today’s 3 guest AI scans are used up.")).toBeVisible();
    await expect(dialog.getByRole("button", { name: /Analyze|Use this photo/ })).toHaveCount(0);
  });
});

test.describe("guest sample photo", () => {
  const SAMPLE_OFFER = /No photo handy\? Try this meal/;

  const sampleAnalysis = [{
    name: "Rice and beans",
    calories: 340,
    protein: 12,
    carbs: 58,
    fat: 6,
    sodium: 410,
    calcium: 90,
    iron: 3.1,
    scanType: "food",
    quantityGrams: 250,
    category: "grains",
    estimatedExpiryDays: 3,
  }];

  const openGuestScanner = async (page: Page) => {
    await enterAsGuest(page);
    await headerButton(page, "Log food").click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Photo" }).click();
    return dialog;
  };

  test("analyzes the bundled photo without the guest supplying one", async ({ page }) => {
    await page.route(ANALYZE_PATH, (route) => route.fulfill({ status: 200, json: sampleAnalysis }));
    const dialog = await openGuestScanner(page);

    await dialog.getByRole("button", { name: SAMPLE_OFFER }).click();

    // The sample lands in the preview like any other pick, so the guest still
    // decides whether to spend a scan on it.
    await expect(page.getByAltText("Preview of the selected meal")).toBeVisible();
    await expect(dialog.getByText("sample-meal.jpg")).toBeVisible();

    const requestPromise = page.waitForRequest(ANALYZE_PATH);
    await dialog.getByRole("button", { name: "Use this photo" }).click();
    const payload = (await requestPromise).postDataJSON() as { type: string; mimeType: string; data: string };
    // The sample is uploaded as image bytes prepared on the device — nothing
    // marks it as a sample, so it exercises the real analysis path.
    expect(payload.type).toBe("image");
    expect(payload.mimeType).toBe("image/jpeg");
    expect(payload.data.length).toBeGreaterThan(100);

    await expect(dialog.getByRole("heading", { name: "Check your meal" })).toBeVisible();
    await expect(dialog.getByLabel("Food name")).toHaveValue("Rice and beans");
  });

  test("offers the sample only for a food scan, and never to a signed-in user", async ({ page }) => {
    const dialog = await openGuestScanner(page);
    await expect(dialog.getByRole("button", { name: SAMPLE_OFFER })).toBeVisible();

    // A plated dinner would read poorly as a barcode-side product shot or as
    // a single ingredient, so the offer is withdrawn for those scans.
    await dialog.getByRole("button", { name: "Product" }).click();
    await expect(dialog.getByRole("button", { name: SAMPLE_OFFER })).toHaveCount(0);
    await dialog.getByRole("button", { name: "Ingredient" }).click();
    await expect(dialog.getByRole("button", { name: SAMPLE_OFFER })).toHaveCount(0);
    await dialog.getByRole("button", { name: "Food", exact: true }).click();
    await expect(dialog.getByRole("button", { name: SAMPLE_OFFER })).toBeVisible();

    // Signed in, the scanner stays uncluttered: these users have their own
    // photos and no allowance to explore with.
    await dialog.getByRole("button", { name: "Close" }).click();
    await navTab(page, "Account").click();
    await content(page).getByRole("button", { name: "Log out" }).click();
    await signIn(page);
    await headerButton(page, "Log food").click();
    const signedInDialog = page.getByRole("dialog");
    await signedInDialog.getByRole("button", { name: "Photo" }).click();
    await expect(signedInDialog.getByRole("button", { name: SAMPLE_OFFER })).toHaveCount(0);
  });

  test("reports a sample that cannot be loaded instead of opening an empty preview", async ({ page }) => {
    // Fail the photo itself, not the module that resolves its URL: the dev
    // server answers the "?import" request with the asset path as JavaScript,
    // and aborting that would break the modal instead of the sample. Matching
    // on a bare substring also survives the "?t=" cache-buster the dev server
    // appends whenever the asset changes.
    await page.route(
      /sample-meal/,
      (route) => (route.request().url().includes("?import") ? route.continue() : route.abort()),
    );
    const dialog = await openGuestScanner(page);

    await dialog.getByRole("button", { name: SAMPLE_OFFER }).click();

    await expect(dialog.getByRole("alert")).toContainText("sample photo could not be loaded");
    await expect(page.getByAltText("Preview of the selected meal")).toBeHidden();
    await expect(dialog.getByRole("button", { name: "Use this photo" })).toHaveCount(0);
  });
});

test.describe("scanned pantry lifecycle", () => {
  test("food scan creates provisional nutrition and pantry stock, then consumption reconciles the remainder to waste", async ({ page }) => {
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [{
          name: "Shared pasta",
          calories: 600,
          protein: 30,
          carbs: 90,
          fat: 14,
          sodium: 800,
          calcium: 120,
          iron: 4,
          scanType: "food",
          quantityGrams: 200,
          category: "prepared meals",
          estimatedExpiryDays: 3,
        }],
      })
    );

    await headerButton(page, "Log food").click();
    await page.getByLabel("What did you eat?").fill("shared pasta");
    await page.getByRole("button", { name: "Analyze description" }).click();
    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeVisible();
    await expect(page.getByRole("spinbutton", { name: /Quantity \(g\) for Shared pasta/ })).toHaveValue("200");

    const scanRequestPromise = page.waitForRequest(INVENTORY_SCAN_PATH);
    await page.getByRole("button", { name: "Add 1 item to today" }).click();
    const scanPayload = scanRequestPromise.then((request) => request.postDataJSON());
    await expect(page.getByText("Scanned item saved.", { exact: true })).toBeVisible();
    expect(await scanPayload).toMatchObject({
      source_type: "food",
      name: "Shared pasta",
      quantity_g: 200,
      nutrients: { calories: 600 },
    });
    await expect(content(page).getByText("600 kcal", { exact: true }).filter({ visible: true })).toBeVisible();

    await tab(page, "Pantry").click();
    const pantryRow = content(page).getByText("Shared pasta", { exact: true }).locator("xpath=ancestor::li");
    await expect(pantryRow).toContainText("200 g remaining");
    await page.getByRole("button", { name: "Record consumption for Shared pasta" }).click();
    await expect(page.getByRole("checkbox", { name: /Discard everything left/ })).toBeChecked();
    await page.getByLabel(/Consumed from Shared pasta/).fill("80");

    const consumeRequestPromise = page.waitForRequest(INVENTORY_CONSUME_PATH);
    await page.getByRole("button", { name: "Save consumption" }).click();
    const consumePayload = consumeRequestPromise.then((request) => request.postDataJSON());
    expect(await consumePayload).toEqual({
      quantity_g: 80,
      discard_remaining: true,
      waste_reason: "leftover_not_eaten",
    });
    await expect(page.getByText("Consumption and waste saved.", { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Recent waste" })).toBeVisible();
    await expect(content(page).getByText("120 g", { exact: true })).toBeVisible();

    await tab(page, "Today").click();
    await expect(content(page).getByText("240 kcal", { exact: true }).filter({ visible: true })).toBeVisible();
    await expect(content(page).getByText("600 kcal", { exact: true }).filter({ visible: true })).toHaveCount(0);
  });

  test("applies a normal 200 lifecycle response when stock expires before consumption", async ({ page }) => {
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [{
          name: "Day-old soup",
          calories: 400,
          protein: 18,
          carbs: 48,
          fat: 14,
          sodium: 900,
          calcium: 110,
          iron: 3,
          scanType: "food",
          quantityGrams: 200,
          category: "prepared meals",
          estimatedExpiryDays: 1,
        }],
      })
    );

    await headerButton(page, "Log food").click();
    await page.getByLabel("What did you eat?").fill("day-old soup");
    await page.getByRole("button", { name: "Analyze description" }).click();
    const scanResponse = page.waitForResponse(INVENTORY_SCAN_PATH);
    await page.getByRole("button", { name: "Add 1 item to today" }).click();
    const scanned = await (await scanResponse).json() as Record<string, any>;
    await expect(content(page).getByText("400 kcal", { exact: true }).filter({ visible: true })).toBeVisible();

    await page.route(INVENTORY_CONSUME_PATH, (route) => {
      const resolvedInventory = { ...scanned.inventory };
      delete resolvedInventory.provisional_meal_id;
      resolvedInventory.quantity_consumed = 0;
      resolvedInventory.quantity_wasted = resolvedInventory.quantity_purchased;
      resolvedInventory.is_wasted = true;
      resolvedInventory.is_resolved = true;
      resolvedInventory.consumed_pct = 0;
      resolvedInventory.wasted_pct = 100;
      const wasteEvent = {
        id: "waste-expired-soup",
        inventory_item_id: resolvedInventory.id,
        food_id: resolvedInventory.food_id,
        food_name: resolvedInventory.name,
        quantity_g: resolvedInventory.quantity_purchased,
        wasted_at: new Date().toISOString(),
        reason: "expired_use_by",
        date_label: "use_by",
        date_status: "1_3_days_after",
        package: resolvedInventory.package,
        spoilage: "unknown",
        classification: "avoidable",
        category: resolvedInventory.category,
        created_at: new Date().toISOString(),
        ...impactForQuantity(resolvedInventory.quantity_purchased),
      };
      return route.fulfill({
        status: 200,
        json: {
          inventory: resolvedInventory,
          deleted_meal_id: scanned.meal.id,
          waste_event: wasteEvent,
        },
      });
    });

    await tab(page, "Pantry").click();
    await page.getByRole("button", { name: "Record consumption for Day-old soup" }).click();
    await page.getByLabel("Consumed from Day-old soup (g)").fill("80");
    await page.getByRole("button", { name: "Save consumption" }).click();

    await expect(page.getByText("Consumption and waste saved.", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Record consumption for Day-old soup" })).toHaveCount(0);
    const history = page.getByRole("heading", { name: "Recent waste" }).locator("xpath=..");
    await expect(history).toContainText("Day-old soup");
    await expect(history).toContainText("expired use by");
    await expect(history).toContainText("200 g");

    await tab(page, "Today").click();
    await expect(content(page).getByText("Day-old soup", { exact: true })).toHaveCount(0);
    await expect(content(page).getByText("400 kcal", { exact: true })).toHaveCount(0);
  });

  test("ingredient scan stays out of Today until partial consumption adds scaled nutrition and leaves stock", async ({ page }) => {
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [{
          name: "Whole carrots",
          calories: 100,
          protein: 2,
          carbs: 24,
          fat: 0.4,
          sodium: 120,
          calcium: 66,
          iron: 0.8,
          scanType: "ingredient",
          quantityGrams: 200,
          category: "vegetables",
          estimatedExpiryDays: 7,
        }],
      })
    );

    await headerButton(page, "Log food").click();
    await page.getByRole("button", { name: "Ingredient", exact: true }).click();
    await page.getByLabel("What ingredient is this?").fill("whole carrots");
    await page.getByRole("button", { name: "Analyze description" }).click();
    await expect(page.getByRole("heading", { name: "Check your ingredient" })).toBeVisible();

    const scanRequestPromise = page.waitForRequest(INVENTORY_SCAN_PATH);
    await page.getByRole("button", { name: "Add 1 item to pantry" }).click();
    expect((await scanRequestPromise).postDataJSON()).toMatchObject({
      source_type: "ingredient",
      quantity_g: 200,
      nutrients: { calories: 100 },
    });
    await expect(content(page).getByText(/Nothing on the table yet/)).toBeVisible();
    await expect(content(page).getByText("Whole carrots", { exact: true })).toHaveCount(0);

    await tab(page, "Pantry").click();
    let pantryRow = content(page).getByText("Whole carrots", { exact: true }).locator("xpath=ancestor::li");
    await expect(pantryRow).toContainText("200 g remaining");
    await page.getByRole("button", { name: "Record consumption for Whole carrots" }).click();
    await expect(page.getByRole("checkbox", { name: /Discard everything left/ })).not.toBeChecked();
    await page.getByLabel(/Consumed from Whole carrots/).fill("50");

    const consumeRequestPromise = page.waitForRequest(INVENTORY_CONSUME_PATH);
    await page.getByRole("button", { name: "Save consumption" }).click();
    expect((await consumeRequestPromise).postDataJSON()).toEqual({
      quantity_g: 50,
      discard_remaining: false,
    });
    pantryRow = content(page).getByText("Whole carrots", { exact: true }).locator("xpath=ancestor::li");
    await expect(pantryRow).toContainText("150 g remaining");
    await expect(page.getByRole("heading", { name: "Recent waste" })).toHaveCount(0);

    await tab(page, "Today").click();
    await expect(content(page).getByText("Whole carrots", { exact: true }).filter({ visible: true })).toBeVisible();
    await expect(content(page).getByText("25 kcal", { exact: true }).filter({ visible: true })).toBeVisible();
  });

  test("waste impact cards total CO2e, virtual water, and tree equivalents from saved events", async ({ page }) => {
    await tab(page, "Pantry").click();

    for (const quantity of [40, 60]) {
      await page.getByRole("button", { name: "Log waste for Baby spinach" }).click();
      await page.getByLabel(/Waste from Baby spinach/).fill(String(quantity));
      await page.getByRole("button", { name: "Log waste", exact: true }).click();
    }

    let impactTotals = page.getByRole("region", { name: "Waste impact totals" });
    await expect(impactTotals.getByText("100 g", { exact: true })).toBeVisible();
    await expect(impactTotals.getByText("0.2 kg", { exact: true })).toBeVisible();
    await expect(impactTotals.getByText("200 L", { exact: true })).toBeVisible();
    await expect(impactTotals.getByText("0.002", { exact: true })).toBeVisible();
    await expect(impactTotals).toContainText("estimated CO₂e from waste");
    await expect(impactTotals).toContainText("estimated virtual water");
    await expect(impactTotals).toContainText("urban tree-year equivalents");

    await page.reload();
    await tab(page, "Pantry").click();
    impactTotals = page.getByRole("region", { name: "Waste impact totals" });
    await expect(impactTotals.getByText("0.2 kg", { exact: true })).toBeVisible();
    await expect(impactTotals.getByText("200 L", { exact: true })).toBeVisible();
    await expect(impactTotals.getByText("0.002", { exact: true })).toBeVisible();
  });

  test("refreshes authenticated expiry ledgers when an open app resumes on a later date", async ({ page }) => {
    await tab(page, "Pantry").click();
    await expect(page.getByRole("button", { name: "Record consumption for Baby spinach" })).toBeVisible();

    const future = new Date();
    future.setDate(future.getDate() + 2);
    const expiredWaste = {
      id: "waste-expired-spinach",
      inventory_item_id: "inventory-spinach",
      food_id: "food-spinach",
      food_name: "Baby spinach",
      quantity_g: 200,
      wasted_at: future.toISOString(),
      reason: "expired_best_before",
      date_label: "best_before",
      date_status: "1_3_days_after",
      package: "unopened",
      spoilage: "unknown",
      classification: "avoidable",
      category: "vegetables",
      created_at: future.toISOString(),
      ...impactForQuantity(200),
    };

    // These are the server's post-reconciliation ledgers. Inventory must be
    // requested first; only then may meals and waste be applied together.
    await page.route(INVENTORY_PATH, (route) =>
      route.fulfill({ status: 200, json: [] })
    );
    await page.route(MEALS_PATH, (route) =>
      route.fulfill({ status: 200, json: [] })
    );
    await page.route(WASTE_PATH, (route) =>
      route.fulfill({ status: 200, json: [expiredWaste] })
    );

    await page.clock.setFixedTime(future);
    const refresh = page.waitForRequest((request) =>
      request.method() === "GET" && new URL(request.url()).pathname === "/api/v1/inventory"
    );
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await refresh;

    await expect(page.getByRole("button", { name: "Record consumption for Baby spinach" })).toHaveCount(0);
    const history = page.getByRole("heading", { name: "Recent waste" }).locator("xpath=..");
    await expect(history).toContainText("Baby spinach");
    await expect(history).toContainText("expired best before");
    await expect(history).toContainText("200 g");
  });

  test("guest expiry removes provisional nutrition and moves remaining stock to waste after midnight", async ({ page }) => {
    await navTab(page, "Account").click();
    await content(page).getByRole("button", { name: "Log out" }).click();
    await page.getByRole("button", { name: "Continue as guest" }).click();

    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [{
          name: "Guest leftovers",
          calories: 480,
          protein: 20,
          carbs: 62,
          fat: 16,
          sodium: 700,
          calcium: 90,
          iron: 3,
          scanType: "food",
          quantityGrams: 200,
          category: "prepared meals",
          estimatedExpiryDays: 1,
        }],
      })
    );

    await headerButton(page, "Log food").click();
    await page.getByLabel("What did you eat?").fill("guest leftovers");
    await page.getByRole("button", { name: "Analyze description" }).click();
    await page.getByRole("button", { name: "Add 1 item to today" }).click();
    await expect(content(page).getByText("480 kcal", { exact: true }).filter({ visible: true })).toBeVisible();

    await tab(page, "Pantry").click();
    await expect(page.getByRole("button", { name: "Record consumption for Guest leftovers" })).toBeVisible();

    const future = new Date();
    future.setDate(future.getDate() + 2);
    await page.clock.setFixedTime(future);
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));

    await expect(page.getByRole("button", { name: "Record consumption for Guest leftovers" })).toHaveCount(0);
    const history = page.getByRole("heading", { name: "Recent waste" }).locator("xpath=..");
    await expect(history).toContainText("Guest leftovers");
    await expect(history).toContainText("expired use by");
    await expect(history).toContainText("200 g");

    await tab(page, "Today").click();
    await expect(content(page).getByText("Guest leftovers", { exact: true })).toHaveCount(0);
    await expect(content(page).getByText("480 kcal", { exact: true })).toHaveCount(0);
  });

  test("keeps pantry and nutrition unchanged when consume fails, then retries only once", async ({ page }) => {
    let failOnce = true;
    await page.route(INVENTORY_CONSUME_PATH, (route) => {
      if (failOnce) {
        failOnce = false;
        return route.fulfill({ status: 503, json: { error: "temporarily unavailable" } });
      }
      return route.fallback();
    });

    await tab(page, "Pantry").click();
    await page.getByRole("button", { name: "Record consumption for Baby spinach" }).click();
    await page.getByLabel("Consumed from Baby spinach (g)").fill("50");
    await page.getByRole("button", { name: "Save consumption" }).click();

    await expect(page.getByRole("alert")).toContainText("The consumption could not be saved");
    let pantryRow = content(page).getByText("Baby spinach", { exact: true }).locator("xpath=ancestor::li");
    await expect(pantryRow).toContainText("200 g remaining");
    await expect(page.getByRole("heading", { name: "Recent waste" })).toHaveCount(0);

    // The form remains open with the original quantity. A successful retry
    // applies one 50 g increment, proving the failed request was not optimistic.
    await expect(page.getByLabel("Consumed from Baby spinach (g)")).toHaveValue("50");
    await page.getByRole("button", { name: "Save consumption" }).click();
    await expect(page.getByText("Consumption saved.", { exact: true })).toBeVisible();
    pantryRow = content(page).getByText("Baby spinach", { exact: true }).locator("xpath=ancestor::li");
    await expect(pantryRow).toContainText("150 g remaining");

    await tab(page, "Today").click();
    await expect(content(page).getByText("Baby spinach", { exact: true }).filter({ visible: true })).toBeVisible();
    await expect(content(page).getByText("11.5 kcal", { exact: true }).filter({ visible: true })).toBeVisible();
  });

  test("waits for every multi-scan request before reconciling a partial failure", async ({ page }) => {
    let slowCommitted = false;
    let failedResponded = false;
    let reconciledBeforeSlowCommit = false;
    const committedInventory: Record<string, any>[] = [];
    const committedMeals: Record<string, any>[] = [];
    const scanAttempts: string[] = [];
    let fastFailuresRemaining = 1;

    await page.route(INVENTORY_PATH, (route) => {
      if (failedResponded && !slowCommitted) reconciledBeforeSlowCommit = true;
      return route.fulfill({ status: 200, json: committedInventory });
    });
    await page.route(MEALS_PATH, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill({ status: 200, json: committedMeals });
    });
    await page.route(WASTE_PATH, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill({ status: 200, json: [] });
    });
    await page.route(INVENTORY_SCAN_PATH, async (route) => {
      const input = route.request().postDataJSON() as Record<string, any>;
      scanAttempts.push(input.name);
      if (input.name === "Fast failure" && fastFailuresRemaining > 0) {
        fastFailuresRemaining -= 1;
        failedResponded = true;
        return route.fulfill({ status: 503, json: { error: "one item failed" } });
      }

      if (input.name === "Slow success") {
        await new Promise((resolve) => setTimeout(resolve, 300));
      }
      const now = new Date().toISOString();
      const suffix = input.name.toLowerCase().replace(/\s+/g, "-");
      const meal = {
        id: `meal-${suffix}`,
        name: input.name,
        consumed_at: input.consumed_at ?? now,
        ...input.nutrients,
      };
      const inventory = {
        id: `inventory-${suffix}`,
        food_id: `food-${suffix}`,
        name: input.name,
        quantity_purchased: input.quantity_g,
        quantity_consumed: 0,
        quantity_wasted: 0,
        use_by_date: input.expiry_date,
        date_label: input.date_label,
        storage: input.storage,
        package: input.package,
        source_type: input.source_type,
        category: input.category,
        expiry_is_estimated: input.expiry_is_estimated,
        nutrition_per_100g: nutrientsPer100g(input.nutrients, input.quantity_g),
        provisional_meal_id: meal.id,
        is_wasted: false,
        is_resolved: false,
        consumed_pct: 0,
        wasted_pct: 0,
        created_at: now,
        updated_at: now,
      };
      committedMeals.push(meal);
      committedInventory.push(inventory);
      if (input.name === "Slow success") slowCommitted = true;
      return route.fulfill({ status: 201, json: { inventory, meal } });
    });
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [
          {
            name: "Slow success",
            calories: 320,
            protein: 14,
            carbs: 42,
            fat: 10,
            sodium: 500,
            calcium: 80,
            iron: 2,
            scanType: "food",
            quantityGrams: 200,
            category: "prepared meals",
            estimatedExpiryDays: 3,
          },
          {
            name: "Fast failure",
            calories: 180,
            protein: 8,
            carbs: 20,
            fat: 7,
            sodium: 300,
            calcium: 50,
            iron: 1,
            scanType: "food",
            quantityGrams: 100,
            category: "prepared meals",
            estimatedExpiryDays: 3,
          },
        ],
      })
    );

    await headerButton(page, "Log food").click();
    await page.getByLabel("What did you eat?").fill("two plates");
    await page.getByRole("button", { name: "Analyze description" }).click();
    await page.getByRole("button", { name: "Add 2 items to today" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("alert")).toContainText("1 item saved");
    await expect(dialog.getByRole("alert")).toContainText("retry sends only it");
    await expect(dialog.getByLabel("Food name")).toHaveCount(1);
    await expect(dialog.getByLabel("Food name")).toHaveValue("Fast failure");
    expect(slowCommitted).toBe(true);
    expect(reconciledBeforeSlowCommit).toBe(false);

    await dialog.getByRole("button", { name: "Add 1 item to today" }).click();
    await expect(dialog).toBeHidden();
    expect(scanAttempts.filter((name) => name === "Slow success")).toHaveLength(1);
    expect(scanAttempts.filter((name) => name === "Fast failure")).toHaveLength(2);
    await expect(content(page).getByText("Slow success", { exact: true }).filter({ visible: true })).toBeVisible();
    await expect(content(page).getByText("Fast failure", { exact: true }).filter({ visible: true })).toBeVisible();
    await tab(page, "Pantry").click();
    await expect(page.getByRole("button", { name: "Record consumption for Slow success" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Record consumption for Fast failure" })).toBeVisible();
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

  test("the guest bypass button enters the app as the guest identity", async ({ page }) => {
    await navTab(page, "Account").click();
    await content(page).getByRole("button", { name: "Log out" }).click();

    // Guest bypass: straight to the dashboard, identity reflects the bypass.
    await page.getByRole("button", { name: "Continue as guest" }).click();
    await expect(page.getByRole("heading", { name: /at the table/ })).toBeVisible();
    await navTab(page, "Account").click();
    await expect(page.getByText("guest@nutri.local")).toBeVisible();

    // A normal sign-in clears the guest session and returns to the demo account.
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
    await expect(page.getByText("of 2,500 kcal", { exact: true }).filter({ visible: true })).toBeVisible();

    await page.reload();
    await expect(page.getByText("of 2,500 kcal", { exact: true }).filter({ visible: true })).toBeVisible();
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

    // Photo mode exposes real camera and gallery buttons in the tab order.
    await page.getByRole("button", { name: "Photo" }).click();
    const upload = page.getByRole("button", { name: "Take a photo" });
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
    await expect(page.getByRole("alert")).toContainText(/couldn’t identify.*food/);
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

test.describe("action affordances", () => {
  test("pantry storage is actionable while unbuilt settings remain honest", async ({ page }) => {
    await tab(page, "Pantry").click();

    const addItem = page.getByRole("button", { name: "Add item" });
    await expect(addItem).toBeEnabled();
    await addItem.click();
    await expect(page.getByLabel("Item name")).toBeVisible();

    // Reminders and sound effects no longer render as switches you can flip.
    await tab(page, "Account").click();
    await expect(page.getByRole("switch", { name: "Dark mode" })).toBeVisible();
    await expect(page.getByRole("switch", { name: /Reminders/ })).toHaveCount(0);
    await expect(page.getByRole("switch", { name: /Sound effects/ })).toHaveCount(0);

    // "View all" pointed at a list that was already fully shown.
    await tab(page, "Today").click();
    await expect(page.getByRole("button", { name: "View all" })).toHaveCount(0);
  });

  test("pantry items and waste history survive refresh", async ({ page }) => {
    await tab(page, "Pantry").click();
    await page.getByRole("button", { name: "Add item" }).click();
    await page.getByLabel("Item name").fill("Fresh tomatoes");
    await page.getByLabel("Quantity (g)").fill("450");
    await page.getByRole("button", { name: "Save item" }).click();
    await expect(page.getByText("Fresh tomatoes")).toBeVisible();

    await page.getByRole("button", { name: "Log waste for Baby spinach" }).click();
    await page.getByLabel(/Waste from Baby spinach/).fill("40");
    await page.getByRole("button", { name: "Log waste", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Recent waste" })).toBeVisible();
    await expect(page.getByText("40 g", { exact: true })).toBeVisible();

    await page.reload();
    await tab(page, "Pantry").click();
    await expect(page.getByText("Fresh tomatoes")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Recent waste" })).toBeVisible();
    await expect(page.getByText("40 g", { exact: true })).toBeVisible();
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

  test("keeps account actions at least 44px tall", async ({ page }) => {
    await tab(page, "Account").click();
    const logout = content(page).getByRole("button", { name: "Log out" });
    await expect(logout).toBeVisible();

    const box = await logout.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
  });

  test("shows Nutri in the Today flow without covering the content", async ({ page }) => {
    const cat = page.getByAltText(/black cat nutrition companion/i);
    await expect(cat).toBeVisible();
    await expect(page.getByText("Small steps, real progress.")).toBeVisible();
    await expect(page.getByRole("button", { name: /nutrition companion/i })).toHaveCount(0);
  });

  test("opens a camera-first full-screen capture flow", async ({ page }) => {
    const trigger = page.getByRole("button", { name: "Scan food", exact: true });
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(48);
    expect(triggerBox!.width).toBeGreaterThanOrEqual(48);

    await trigger.click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(page.getByRole("button", { name: "Take a photo", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Choose from gallery" })).toBeVisible();
    await expect(page.locator('input[type="file"][capture="environment"]')).toHaveCount(1);
  });

  test("uploads a prepared photo, allows corrections, and updates Today", async ({ page }) => {
    await page.route(ANALYZE_PATH, (route) =>
      route.fulfill({
        status: 200,
        json: [{
          name: "Grain bowl",
          calories: 540,
          protein: 24,
          carbs: 68,
          fat: 19,
          sodium: 620,
          calcium: 180,
          iron: 4.2,
        }],
      })
    );

    await page.getByRole("button", { name: "Scan food", exact: true }).click();
    await page.locator('input[type="file"]').nth(1).setInputFiles("src/assets/photo.jpg");
    await expect(page.getByAltText("Preview of the selected meal")).toBeVisible();

    const requestPromise = page.waitForRequest(ANALYZE_PATH);
    await page.getByRole("button", { name: "Use this photo" }).click();
    const request = await requestPromise;
    const payload = request.postDataJSON() as { type: string; mimeType: string; data: string };
    expect(payload.type).toBe("image");
    expect(payload.mimeType).toBe("image/jpeg");
    expect(payload.data.length).toBeGreaterThan(100);

    await expect(page.getByRole("heading", { name: "Check your meal" })).toBeVisible();
    await page.getByLabel("Food name").fill("Garden grain bowl");
    await page.getByRole("spinbutton", { name: /Calories for/ }).fill("560");
    await page.getByRole("button", { name: "Add 1 item to today" }).click();

    await expect(page.getByText("Scanned item saved.", { exact: true })).toBeVisible();
    const recentMeals = page.getByRole("region", { name: "Recent meals" });
    await expect(recentMeals.getByText("Garden grain bowl")).toBeVisible();
    await expect(recentMeals.getByText("560 kcal")).toBeVisible();
  });
});
