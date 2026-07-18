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

// One item returned by food analysis — the nutrient breakdown plus a name,
// ready to become a FoodEntry once an id and timestamp are attached.
export type AnalyzedFoodItem = NutritionData & { name: string };

// Guest roles offered by the sign-in screen's auth bypass (testing only).
export type GuestRole = "developer" | "alpha";

// Presentational identity for the signed-in user. There is no real auth yet;
// the default profile is the demo account and guest roles swap it out so the
// bypass is visible in the chrome and on the account page.
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
