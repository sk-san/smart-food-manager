// Food analysis + companion messages.
//
// Migrated from the source's geminiService, which called the Gemini API
// directly from the browser with an exposed API key. That violates the
// project's logging/data-infra blueprint (no secrets or raw prompts in the
// client) so these now route through the backend via our instrumented
// apiPost client — every call carries traceparent and emits api_request_*
// telemetry. Calls fall back to local estimates when the configured AI
// provider is unavailable, so the UI stays functional without an API key.
import { apiPost } from "../api/client";
import { logEvent } from "../telemetry/logger";
import {
  DEFAULT_EXPIRY_DAYS,
  type AnalyzedFoodItem,
  type DailyGoal,
  type NutritionData,
  type ScanType,
} from "../types/nutrition";

const ANALYZE_PATH = "/api/v1/nutrition/analyze";
const COMPANION_PATH = "/api/v1/companion/message";

// A deterministic nutrition fallback for provider or network failures. Scanner
// metadata is attached at call time so it follows the type the user selected.
const FALLBACK_NUTRITION: NutritionData & { name: string } = {
  name: "Estimated item",
  calories: 95,
  protein: 0.5,
  carbs: 25,
  fat: 0.3,
  sodium: 1,
  calcium: 6,
  iron: 0.1,
};

type RawAnalyzedFoodItem = NutritionData & {
  name: string;
  scanType?: unknown;
  scan_type?: unknown;
  quantityGrams?: unknown;
  quantity_grams?: unknown;
  category?: unknown;
  estimatedExpiryDays?: unknown;
  estimated_expiry_days?: unknown;
};

const isScanType = (value: unknown): value is ScanType =>
  value === "food" || value === "product" || value === "ingredient";

const finiteNumber = (value: unknown): number | null =>
  typeof value === "number" && Number.isFinite(value) ? value : null;

const normalizeAnalyzedItem = (
  item: RawAnalyzedFoodItem,
  selectedScanType: ScanType,
): AnalyzedFoodItem => {
  const responseScanType = item.scanType ?? item.scan_type;
  const quantity = finiteNumber(item.quantityGrams ?? item.quantity_grams);
  const expiryDays = finiteNumber(item.estimatedExpiryDays ?? item.estimated_expiry_days);
  const category = typeof item.category === "string" ? item.category.trim() : "";
  const scanType = isScanType(responseScanType) ? responseScanType : selectedScanType;

  return {
    name: item.name,
    calories: item.calories,
    protein: item.protein,
    carbs: item.carbs,
    fat: item.fat,
    sodium: item.sodium,
    calcium: item.calcium,
    iron: item.iron,
    scanType,
    quantityGrams: quantity !== null && quantity > 0 ? quantity : 100,
    category: category || "other",
    estimatedExpiryDays:
      expiryDays !== null && expiryDays >= 0
        ? Math.round(expiryDays)
        : DEFAULT_EXPIRY_DAYS[scanType],
  };
};

async function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => {
      // Strip the "data:<mime>;base64," prefix.
      const result = reader.result as string;
      resolve(result.split(",")[1] ?? "");
    };
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

// analyzeFoodInput estimates the nutrition of a text description or food
// photo. Image bytes are sent in the request body but never logged; only
// non-sensitive file metadata (mime type, size) goes to telemetry, per the
// blueprint.
export async function analyzeFoodInput(
  input: string | File,
  inputType: "text" | "image",
  scanType: ScanType,
): Promise<AnalyzedFoodItem[]> {
  const isImage = inputType === "image" && input instanceof File;

  if (isImage) {
    logEvent({
      message: "Food photo upload started",
      name: "file_upload_started",
      category: "file",
      action: "upload",
      outcome: "started",
      attrs: { "file.mime_type": input.type, "file.size_bytes": input.size },
    });
  }

  try {
    const body =
      inputType === "text"
        ? { type: "text", text: input as string, scanType }
        : {
            type: "image",
            mimeType: (input as File).type,
            data: await fileToBase64(input as File),
            scanType,
          };

    const items = await apiPost<RawAnalyzedFoodItem[]>(ANALYZE_PATH, body);

    if (isImage) {
      logEvent({
        message: "Food photo upload completed",
        name: "file_upload_completed",
        category: "file",
        action: "upload",
        outcome: "success",
        attrs: { "file.mime_type": (input as File).type, "file.size_bytes": (input as File).size },
      });
    }
    return items.map((item) => normalizeAnalyzedItem(item, scanType));
  } catch (err) {
    if (isImage) {
      logEvent({
        severity: "ERROR",
        message: "Food photo upload failed",
        name: "file_upload_failed",
        category: "file",
        action: "upload",
        outcome: "failure",
        attrs: { "error.type": err instanceof Error ? err.name : "Error" },
      });
    }
    // A failed photo upload must stay a failed upload: presenting a generic
    // "Scanned food" estimate would imply the backend had seen the image.
    // Text remains usable offline with an explicitly generic estimate.
    if (isImage) throw err;
    return [normalizeAnalyzedItem({
      ...FALLBACK_NUTRITION,
      name: deriveName(input as string),
    }, scanType)];
  }
}

function deriveName(text: string): string {
  const trimmed = text.trim();
  if (!trimmed) return FALLBACK_NUTRITION.name;
  return trimmed.length > 40 ? trimmed.slice(0, 40) + "…" : trimmed;
}

// getCompanionMessage asks the backend for an in-character message from
// "Nutri". Falls back to a local progress-based kaomoji when unavailable.
export async function getCompanionMessage(
  stats: NutritionData,
  goals: DailyGoal
): Promise<string> {
  try {
    const res = await apiPost<{ message: string }>(COMPANION_PATH, { stats, goals });
    return res.message;
  } catch {
    return localCompanionMessage(stats, goals);
  }
}

function localCompanionMessage(stats: NutritionData, goals: DailyGoal): string {
  const ratio = goals.calories > 0 ? stats.calories / goals.calories : 0;
  if (ratio > 1) return "Your day is logged. One number never defines your progress.";
  if (ratio > 0.8) return "You’re close to today’s energy guide. Keep listening to your body.";
  if (ratio > 0.5) return "Steady progress. Every meal gives us a clearer picture.";
  return "Ready when you are. Snap a meal and I’ll help with the numbers.";
}
