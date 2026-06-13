// Food analysis + companion messages.
//
// Migrated from the source's geminiService, which called the Gemini API
// directly from the browser with an exposed API key. That violates the
// project's logging/data-infra blueprint (no secrets or raw prompts in the
// client) so these now route through the backend via our instrumented
// apiPost client — every call carries traceparent and emits api_request_*
// telemetry. Until the backend AI route ships, calls fall back to a local
// estimate so the UI stays functional.
import { apiPost } from "../api/client";
import { logEvent } from "../telemetry/logger";
import type { AnalyzedFoodItem, DailyGoal, NutritionData } from "../types/nutrition";

const ANALYZE_PATH = "/api/v1/nutrition/analyze";
const COMPANION_PATH = "/api/v1/companion/message";

// A deterministic placeholder so the log screen works end to end before the
// backend analysis endpoint exists.
const FALLBACK_ITEM: AnalyzedFoodItem = {
  name: "Estimated item",
  calories: 95,
  protein: 0.5,
  carbs: 25,
  fat: 0.3,
  sodium: 1,
  calcium: 6,
  iron: 0.1,
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
  inputType: "text" | "image"
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
        ? { type: "text", text: input as string }
        : {
            type: "image",
            mimeType: (input as File).type,
            data: await fileToBase64(input as File),
          };

    const items = await apiPost<AnalyzedFoodItem[]>(ANALYZE_PATH, body);

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
    return items;
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
    // Backend route not available yet (or transient failure): keep the UI
    // usable with a single estimated item.
    return [{ ...FALLBACK_ITEM, name: inputType === "text" ? deriveName(input as string) : "Scanned food" }];
  }
}

function deriveName(text: string): string {
  const trimmed = text.trim();
  if (!trimmed) return FALLBACK_ITEM.name;
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
  if (ratio > 1) return "Oof! Too full! ( >ω<)";
  if (ratio > 0.8) return "Almost there, Master! (◕‿◕)";
  if (ratio > 0.5) return "Yummy progress! :3";
  return "I'm hungry... feed me! (・`ω´・)";
}
