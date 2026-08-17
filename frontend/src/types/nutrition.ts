// Domain types for the NutriMind nutrition tracker UI.

export interface NutritionData {
  calories: number;
  protein: number;
  carbs: number;
  fat: number;
  sodium: number; // mg
  calcium: number; // mg
  iron: number; // mg
}

export interface FoodEntry extends NutritionData {
  id: string;
  name: string;
  timestamp: number;
  icon?: string; // emoji or icon name
}

export interface DailyGoal {
  calories: number;
  protein: number;
  carbs: number;
  fat: number;
  sodium: number;
  calcium: number;
  iron: number;
}

/** What the scanner is looking at, and therefore how it affects the ledger. */
export type ScanType = "food" | "product" | "ingredient";

/** Storage choices shared by scanner review and the pantry. */
export type FoodStorage = "fridge" | "freezer" | "pantry" | "other";

/**
 * Conservative shelf-life estimates used only when analysis cannot provide a
 * better value. The review screen always turns the estimate into an editable
 * calendar date before anything is saved.
 */
export const DEFAULT_EXPIRY_DAYS: Record<ScanType, number> = {
  food: 3,
  product: 30,
  ingredient: 7,
};

// One item returned by scanner analysis. Nutrients describe the full scanned
// quantity. Food and product nutrition is provisional until consumption is
// reconciled; ingredient nutrition is recorded only when it is consumed.
export type AnalyzedFoodItem = NutritionData & {
  name: string;
  scanType: ScanType;
  quantityGrams: number;
  category: string;
  estimatedExpiryDays: number;
};

/** Reviewed scanner output, ready for the meal/pantry persistence workflow. */
export type ScannedFoodInput = AnalyzedFoodItem & {
  expiryDate: string;
  /** True until the user explicitly edits the AI/default expiry date. */
  expiryIsEstimated: boolean;
  storage: FoodStorage;
};

/**
 * A multi-item save can succeed per item. Keeping the failed indexes lets the
 * review screen remove committed items so a retry cannot duplicate them.
 */
export class PartialScanSaveError extends Error {
  constructor(
    public readonly failedIndexes: number[],
    public readonly succeededCount: number,
  ) {
    super("Some scanned items were saved while others failed.");
    this.name = "PartialScanSaveError";
  }
}

// Guest roles offered by the sign-in screen's auth bypass (testing only).
export type GuestRole = "developer" | "alpha";

// Presentational identity for the signed-in user. The auth endpoint currently
// returns only a token, so the default account chrome remains static; guest
// roles swap it out so the testing bypass is visible.
export interface UserProfile {
  name: string;
  email: string;
  initials: string;
  tag: string;
}

export const DEFAULT_PROFILE: UserProfile = {
  name: "John Doe",
  email: "john.doe@example.com",
  initials: "JD",
  tag: "since March",
};

export const GUEST_PROFILES: Record<GuestRole, UserProfile> = {
  developer: {
    name: "Developer",
    email: "dev@nutri.local",
    initials: "DV",
    tag: "internal build",
  },
  alpha: {
    name: "Alpha Tester",
    email: "alpha@nutri.local",
    initials: "AT",
    tag: "alpha cohort",
  },
};

// Demo account aggregates shown on the account page and the almanac board —
// real aggregation is a follow-up once entries persist through the backend.
export const DEMO_ACCOUNT_STATS = {
  daysLogged: 196,
  avgCalories: 2090,
  bestStreak: 12,
  currentStreak: 5,
};

// The app's suggested daily targets — the initial goals in App and what the
// "Reset to suggested" action in the account view restores.
export const SUGGESTED_GOALS: DailyGoal = {
  calories: 2200,
  protein: 150,
  carbs: 250,
  fat: 70,
  sodium: 2300,
  calcium: 1000,
  iron: 18,
};
