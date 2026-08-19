export interface Health {
  status: string;
  db: boolean;
  time: string;
}

export interface Nutrient {
  id: number;
  code: string;
  name: string;
  unit: string;
  focus: "deficiency_watch" | "excess_watch" | "caution";
  reference_daily_amount: number | null;
}

export interface LoginResponse {
  token: string;
}

export interface CurrentUserResponse {
  user_id: string;
  roles: string[];
  email: string;
  /** Never blank: the backend falls back to the email's local part. */
  display_name: string;
}

export interface MealResponse {
  id: string;
  name: string;
  consumed_at: string;
  calories: number;
  protein: number;
  carbs: number;
  fat: number;
  sodium: number;
  calcium: number;
  iron: number;
}

export type InventorySourceType = "food" | "product" | "ingredient";

export interface NutritionAmountsResponse {
  calories: number;
  protein: number;
  carbs: number;
  fat: number;
  sodium: number;
  calcium: number;
  iron: number;
}

export interface InventoryResponse {
  id: string;
  food_id: string;
  name: string;
  quantity_purchased: number;
  quantity_consumed: number;
  quantity_wasted: number;
  purchase_date?: string;
  best_before_date?: string;
  use_by_date?: string;
  date_label: string;
  storage: string;
  package: string;
  is_wasted: boolean;
  is_resolved: boolean;
  consumed_pct: number;
  wasted_pct: number;
  created_at: string;
  updated_at: string;
  source_type?: InventorySourceType;
  category?: string;
  provisional_meal_id?: string | null;
  expiry_is_estimated?: boolean;
  nutrition_per_100g?: NutritionAmountsResponse | null;
}

export interface WasteEventResponse {
  id: string;
  inventory_item_id?: string;
  food_id: string;
  food_name: string;
  quantity_g: number;
  wasted_at: string;
  reason: string;
  date_label: string;
  date_status: string;
  package: string;
  spoilage: string;
  classification: string;
  note?: string;
  created_at: string;
  category?: string;
  /** Current backend contract. `kg_co2e` is accepted by the mapper for compatibility. */
  impact_kg_co2e?: number;
  kg_co2e?: number;
  virtual_water_l?: number;
  tree_equivalents?: number;
  impact_factor_version?: string;
}

export interface InventoryScanRequest {
  source_type: InventorySourceType;
  name: string;
  category?: string;
  quantity_g: number;
  expiry_date?: string;
  expiry_is_estimated: boolean;
  date_label: "best_before" | "use_by";
  storage: "fridge" | "freezer" | "pantry" | "other";
  package: "unopened" | "opened" | "cooked" | "leftover" | "unknown";
  consumed_at: string;
  nutrients: NutritionAmountsResponse;
}

export interface InventoryScanResponse {
  inventory: InventoryResponse;
  meal?: MealResponse;
}

export interface InventoryConsumeResponse {
  inventory: InventoryResponse;
  meal?: MealResponse;
  deleted_meal_id?: string;
  waste_event?: WasteEventResponse;
}
