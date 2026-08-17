import { apiGet, apiPost, apiPut } from '../api/client';
import {
  InventoryConsumeResponse,
  InventoryResponse,
  InventoryScanRequest,
  InventoryScanResponse,
  MealResponse,
  WasteEventResponse,
} from '../api/types';
import { DailyGoal, FoodEntry, ScannedFoodInput } from '../types/nutrition';
import { LarderItem, NewLarderItem, WasteEvent } from '../types/pantry';

const MEALS_PATH = '/api/v1/meals';
const GOALS_PATH = '/api/v1/goals';
const INVENTORY_PATH = '/api/v1/inventory';
const WASTE_PATH = '/api/v1/waste-events';
const INVENTORY_SCANS_PATH = `${INVENTORY_PATH}/scans`;

export interface InventoryScanResult {
  inventory: LarderItem;
  meal?: FoodEntry;
}

export interface InventoryConsumptionResult extends InventoryScanResult {
  deletedMealId?: string;
  wasteEvent?: WasteEvent;
}

const mealToEntry = (meal: MealResponse): FoodEntry => ({
  id: meal.id,
  name: meal.name,
  timestamp: new Date(meal.consumed_at).getTime(),
  calories: meal.calories,
  protein: meal.protein,
  carbs: meal.carbs,
  fat: meal.fat,
  sodium: meal.sodium,
  calcium: meal.calcium,
  iron: meal.iron,
});

const entryPayload = (entry: Omit<FoodEntry, 'id'>) => ({
  name: entry.name,
  consumed_at: new Date(entry.timestamp).toISOString(),
  calories: entry.calories,
  protein: entry.protein,
  carbs: entry.carbs,
  fat: entry.fat,
  sodium: entry.sodium,
  calcium: entry.calcium,
  iron: entry.iron,
});

export async function listMeals(): Promise<FoodEntry[]> {
  return (await apiGet<MealResponse[]>(MEALS_PATH)).map(mealToEntry);
}

export async function createMeal(entry: Omit<FoodEntry, 'id'>): Promise<FoodEntry> {
  return mealToEntry(await apiPost<MealResponse>(MEALS_PATH, entryPayload(entry)));
}

export async function updateMeal(entry: FoodEntry): Promise<FoodEntry> {
  return mealToEntry(await apiPut<MealResponse>(`${MEALS_PATH}/${entry.id}`, entryPayload(entry)));
}

export const getGoals = () => apiGet<DailyGoal>(GOALS_PATH);
export const putGoals = (goals: DailyGoal) => apiPut<DailyGoal>(GOALS_PATH, goals);

const expiryDays = (date?: string) => {
  if (!date) return 30;
  const today = new Date();
  const [year, month, day] = date.split('-').map(Number);
  if (![year, month, day].every(Number.isFinite)) return 30;
  const todayOrdinal = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  const expiryOrdinal = Date.UTC(year, month - 1, day);
  return Math.round((expiryOrdinal - todayOrdinal) / 86_400_000);
};

const monogram = (name: string) => {
  const words = name.trim().split(/\s+/);
  return words.length > 1
    ? `${words[0][0]}${words[1][0]}`.toUpperCase()
    : name.slice(0, 2).replace(/^./, (letter) => letter.toUpperCase());
};

const inventoryToLarder = (item: InventoryResponse): LarderItem => {
  const expiryDate = item.use_by_date || item.best_before_date;
  return {
    id: item.id,
    foodId: item.food_id,
    name: item.name,
    label: item.name.split(/\s+/)[0].toLowerCase(),
    monogram: monogram(item.name),
    daysLeft: expiryDays(expiryDate),
    quantityPurchased: item.quantity_purchased,
    quantityConsumed: item.quantity_consumed,
    quantityWasted: item.quantity_wasted,
    storage: item.storage,
    package: item.package,
    dateLabel: item.date_label,
    expiryDate,
    expiryIsEstimated: item.expiry_is_estimated ?? false,
    sourceType: item.source_type ?? 'ingredient',
    category: item.category || 'other',
    provisionalMealId: item.provisional_meal_id || undefined,
    nutritionPer100g: item.nutrition_per_100g ?? undefined,
    isResolved: item.is_resolved,
  };
};

export async function listInventory(): Promise<LarderItem[]> {
  return (await apiGet<InventoryResponse[]>(INVENTORY_PATH))
    .filter((item) => !item.is_resolved)
    .map(inventoryToLarder);
}

export async function createInventoryItem(input: NewLarderItem): Promise<LarderItem> {
  const dateLabel = input.expiryDate ? 'best_before' : 'unknown';
  const response = await apiPost<InventoryResponse>(INVENTORY_PATH, {
    name: input.name,
    quantity_purchased: input.quantityPurchased,
    quantity_consumed: 0,
    best_before_date: input.expiryDate,
    date_label: dateLabel,
    storage: input.storage,
    package: 'unopened',
  });
  return inventoryToLarder(response);
}

export const scanExpiryIsEstimated = (input: ScannedFoodInput) => input.expiryIsEstimated;

export const scanNutritionPer100g = (input: ScannedFoodInput) => {
  const scale = 100 / input.quantityGrams;
  return {
    calories: input.calories * scale,
    protein: input.protein * scale,
    carbs: input.carbs * scale,
    fat: input.fat * scale,
    sodium: input.sodium * scale,
    calcium: input.calcium * scale,
    iron: input.iron * scale,
  };
};

const scannedInventoryPayload = (input: ScannedFoodInput): InventoryScanRequest => ({
  source_type: input.scanType,
  name: input.name,
  category: input.category || undefined,
  quantity_g: input.quantityGrams,
  ...(input.expiryDate ? { expiry_date: input.expiryDate } : {}),
  expiry_is_estimated: scanExpiryIsEstimated(input),
  date_label: input.scanType === 'product' ? 'best_before' : 'use_by',
  storage: input.storage,
  package: input.scanType === 'food' ? 'cooked' : 'unopened',
  consumed_at: new Date().toISOString(),
  // Analysis values describe the scanned portion. The backend snapshots and
  // normalizes them after it knows the portion weight.
  nutrients: {
    calories: input.calories,
    protein: input.protein,
    carbs: input.carbs,
    fat: input.fat,
    sodium: input.sodium,
    calcium: input.calcium,
    iron: input.iron,
  },
});

export async function createScannedInventory(input: ScannedFoodInput): Promise<InventoryScanResult> {
  const response = await apiPost<InventoryScanResponse>(INVENTORY_SCANS_PATH, scannedInventoryPayload(input));
  return {
    inventory: inventoryToLarder(response.inventory),
    meal: response.meal ? mealToEntry(response.meal) : undefined,
  };
}

const wasteToDomain = (event: WasteEventResponse): WasteEvent => ({
  id: event.id,
  inventoryItemId: event.inventory_item_id,
  foodName: event.food_name,
  quantity: event.quantity_g,
  wastedAt: new Date(event.wasted_at).getTime(),
  reason: event.reason,
  category: event.category,
  impactKgCO2e: event.impact_kg_co2e ?? event.kg_co2e ?? 0,
  virtualWaterL: event.virtual_water_l ?? 0,
  treeEquivalents: event.tree_equivalents ?? 0,
  impactFactorVersion: event.impact_factor_version,
});

export async function listWasteEvents(): Promise<WasteEvent[]> {
  return (await apiGet<WasteEventResponse[]>(WASTE_PATH)).map(wasteToDomain);
}

export async function createWasteEvent(
  item: LarderItem,
  quantity: number,
  reason: string,
): Promise<WasteEvent> {
  const response = await apiPost<WasteEventResponse>(WASTE_PATH, {
    inventory_item_id: item.id,
    quantity_g: quantity,
    reason,
    date_label: item.dateLabel || (item.expiryDate ? 'best_before' : 'unknown'),
    package: item.package || 'unknown',
  });
  return wasteToDomain(response);
}

export async function consumeInventoryItem(
  item: LarderItem,
  quantity: number,
  discardRemaining: boolean,
  wasteReason?: string,
): Promise<InventoryConsumptionResult> {
  const response = await apiPost<InventoryConsumeResponse>(`${INVENTORY_PATH}/${item.id}/consume`, {
    quantity_g: quantity,
    discard_remaining: discardRemaining,
    ...(discardRemaining && wasteReason ? { waste_reason: wasteReason } : {}),
  });
  return {
    inventory: inventoryToLarder(response.inventory),
    meal: response.meal ? mealToEntry(response.meal) : undefined,
    deletedMealId: response.deleted_meal_id,
    wasteEvent: response.waste_event ? wasteToDomain(response.waste_event) : undefined,
  };
}
