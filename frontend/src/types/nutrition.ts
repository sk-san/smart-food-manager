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
