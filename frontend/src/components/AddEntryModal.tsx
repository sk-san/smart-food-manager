import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowLeft,
  Camera,
  Carrot,
  Check,
  Image as ImageIcon,
  Loader2,
  Package,
  Plus,
  RotateCcw,
  ShieldCheck,
  Sparkles,
  Trash2,
  Type,
  X,
} from "lucide-react";
import { analyzeFoodInput } from "../services/nutritionService";
import { PhotoValidationError, prepareFoodPhoto, validateFoodPhoto } from "../services/imageProcessing";
import {
  DEFAULT_EXPIRY_DAYS,
  type FoodStorage,
  PartialScanSaveError,
  type ScannedFoodInput,
  type ScanType,
} from "../types/nutrition";

interface AddEntryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAdd: (items: ScannedFoodInput[]) => void | Promise<void>;
}

type InputMode = "text" | "image";
type AnalysisPhase = "idle" | "preparing" | "uploading" | "analyzing";
type EditableItem = ScannedFoodInput;
type NumericItemKey =
  | "quantityGrams"
  | "calories"
  | "protein"
  | "carbs"
  | "fat"
  | "sodium"
  | "calcium"
  | "iron";

const TITLE_ID = "log-food-title";
const TEXT_INPUT_ID = "log-food-description";
const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "textarea:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

const defaultMode = (): InputMode =>
  window.matchMedia("(max-width: 767px)").matches ? "image" : "text";

const formatFileSize = (bytes: number) =>
  bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB`;

const SCAN_TYPE_OPTIONS: Array<{
  value: ScanType;
  label: string;
  icon: typeof Sparkles;
}> = [
  { value: "food", label: "Food", icon: Sparkles },
  { value: "product", label: "Product", icon: Package },
  { value: "ingredient", label: "Ingredient", icon: Carrot },
];

const SCAN_TYPE_COPY: Record<ScanType, {
  behavior: string;
  photoHeading: string;
  photoHint: string;
  textLabel: string;
  textPlaceholder: string;
  reviewTitle: string;
}> = {
  food: {
    behavior: "Nutrition counts provisionally today, and the food is added to your pantry. You can correct the amount eaten later.",
    photoHeading: "What’s on your plate?",
    photoHint: "Get the whole meal in frame. Natural light and a top-down angle help Nutri estimate portions.",
    textLabel: "What did you eat?",
    textPlaceholder: "e.g., avocado toast with two eggs and cherry tomatoes",
    reviewTitle: "Check your meal",
  },
  product: {
    behavior: "Nutrition counts provisionally today, and the packaged product is added to your pantry for later reconciliation.",
    photoHeading: "Show the product clearly",
    photoHint: "Keep the product name and nutrition label readable. You can correct every estimate before saving.",
    textLabel: "What product is this?",
    textPlaceholder: "e.g., 400 g strawberry yogurt with 95 kcal per 100 g",
    reviewTitle: "Check your product",
  },
  ingredient: {
    behavior: "The ingredient goes to your pantry now. Its nutrition is counted only when you record consuming it.",
    photoHeading: "Show the ingredient",
    photoHint: "Include the full ingredient and its approximate amount so Nutri can estimate quantity and shelf life.",
    textLabel: "What ingredient is this?",
    textPlaceholder: "e.g., 250 g fresh baby spinach",
    reviewTitle: "Check your ingredient",
  },
};

const NUTRIENT_FIELDS: Array<{
  key: Exclude<NumericItemKey, "quantityGrams">;
  label: string;
  unit: "kcal" | "g" | "mg";
}> = [
  { key: "calories", label: "Calories", unit: "kcal" },
  { key: "protein", label: "Protein", unit: "g" },
  { key: "carbs", label: "Carbs", unit: "g" },
  { key: "fat", label: "Fat", unit: "g" },
  { key: "sodium", label: "Sodium", unit: "mg" },
  { key: "calcium", label: "Calcium", unit: "mg" },
  { key: "iron", label: "Iron", unit: "mg" },
];

const expiryDateFromDays = (days: number): string => {
  const date = new Date();
  date.setHours(12, 0, 0, 0);
  date.setDate(date.getDate() + Math.max(0, Math.round(days)));
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
};

const defaultStorage = (scanType: ScanType, category = "other"): FoodStorage => {
  const normalized = category.toLowerCase();
  if (normalized.includes("frozen")) return "freezer";
  if (
    normalized.includes("dairy") ||
    normalized.includes("fresh") ||
    normalized.includes("chilled") ||
    normalized.includes("refrigerated") ||
    normalized.includes("meat") ||
    normalized.includes("seafood")
  ) {
    return "fridge";
  }
  if (
    normalized.includes("shelf") ||
    normalized.includes("dry") ||
    normalized.includes("canned") ||
    normalized.includes("grain") ||
    normalized.includes("pasta") ||
    normalized.includes("spice") ||
    scanType === "product"
  ) {
    return "pantry";
  }
  return "fridge";
};

const emptyItem = (scanType: ScanType): EditableItem => ({
  name: "",
  calories: 0,
  protein: 0,
  carbs: 0,
  fat: 0,
  sodium: 0,
  calcium: 0,
  iron: 0,
  scanType,
  quantityGrams: 100,
  category: "other",
  estimatedExpiryDays: DEFAULT_EXPIRY_DAYS[scanType],
  expiryDate: expiryDateFromDays(DEFAULT_EXPIRY_DAYS[scanType]),
  expiryIsEstimated: true,
  storage: defaultStorage(scanType),
});

const AddEntryModal: React.FC<AddEntryModalProps> = ({ isOpen, onClose, onAdd }) => {
  const [mode, setMode] = useState<InputMode>(defaultMode);
  const [scanType, setScanType] = useState<ScanType>("food");
  const [inputText, setInputText] = useState("");
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [analysisPhase, setAnalysisPhase] = useState<AnalysisPhase>("idle");
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [analysisResult, setAnalysisResult] = useState<EditableItem[] | null>(null);

  const cameraInputRef = useRef<HTMLInputElement>(null);
  const galleryInputRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  const reset = useCallback(() => {
    setInputText("");
    setSelectedFile(null);
    setPreviewUrl(null);
    setAnalysisResult(null);
    setError(null);
    setAnalysisPhase("idle");
    setIsSaving(false);
    setMode(defaultMode());
    setScanType("food");
    if (cameraInputRef.current) cameraInputRef.current.value = "";
    if (galleryInputRef.current) galleryInputRef.current.value = "";
  }, []);

  const handleClose = useCallback(() => {
    reset();
    onClose();
  }, [onClose, reset]);

  useEffect(() => {
    if (!isOpen) return;
    const opener = document.activeElement as HTMLElement | null;
    dialogRef.current?.focus();
    return () => {
      if (opener?.isConnected) opener.focus();
    };
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        handleClose();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isOpen, handleClose]);

  useEffect(() => {
    if (!isOpen) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [isOpen]);

  useEffect(() => {
    if (!previewUrl) return;
    return () => URL.revokeObjectURL(previewUrl);
  }, [previewUrl]);

  if (!isOpen) return null;

  const isAnalyzing = analysisPhase !== "idle";

  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;

    try {
      validateFoodPhoto(file);
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
      setMode("image");
      setAnalysisResult(null);
      setError(null);
    } catch (selectionError) {
      setSelectedFile(null);
      setPreviewUrl(null);
      setError(
        selectionError instanceof PhotoValidationError
          ? selectionError.message
          : "That photo could not be opened. Choose another one."
      );
    }
  };

  const handleAnalyze = async () => {
    if (mode === "text" && !inputText.trim()) return;
    if (mode === "image" && !selectedFile) return;

    setAnalysisResult(null);
    setError(null);

    try {
      let input: string | File;
      if (mode === "image") {
        setAnalysisPhase("preparing");
        input = await prepareFoodPhoto(selectedFile!);
        setAnalysisPhase("uploading");
      } else {
        input = inputText.trim();
        setAnalysisPhase("analyzing");
      }

      const result = await analyzeFoodInput(input, mode, scanType);
      if (result.length === 0) {
        setError(
          mode === "image"
            ? `We couldn’t identify this ${scanType} in the photo. Try brighter light or add a short description.`
            : `We couldn’t identify this ${scanType} in the description. Add a little more detail and try again.`
        );
        return;
      }
      setAnalysisResult(result.map((item) => ({
        ...item,
        expiryDate: expiryDateFromDays(item.estimatedExpiryDays),
        expiryIsEstimated: true,
        storage: defaultStorage(item.scanType, item.category),
      })));
    } catch (analysisError) {
      if (analysisError instanceof PhotoValidationError) {
        setError(analysisError.message);
      } else {
        setError(
          mode === "image"
            ? "The photo wasn’t uploaded. It’s still here—check your connection and try again."
            : `That ${scanType} couldn’t be analyzed. Check your connection and try again.`
        );
      }
    } finally {
      setAnalysisPhase("idle");
    }
  };

  const handleConfirm = async () => {
    if (
      !analysisResult?.length ||
      analysisResult.some((item) => !item.name.trim() || item.quantityGrams <= 0 || !item.expiryDate)
    ) return;
    setIsSaving(true);
    setError(null);
    try {
      await onAdd(analysisResult.map((item) => ({
        ...item,
        name: item.name.trim(),
        category: item.category.trim() || "other",
      })));
      handleClose();
    } catch (saveError) {
      console.error(saveError);
      if (saveError instanceof PartialScanSaveError) {
        const failed = new Set(saveError.failedIndexes);
        setAnalysisResult((current) => current?.filter((_, index) => failed.has(index)) ?? null);
        const savedLabel = `${saveError.succeededCount} ${saveError.succeededCount === 1 ? "item" : "items"} saved.`;
        const failedLabel = `${saveError.failedIndexes.length === 1 ? "The unsaved item remains" : "Only the unsaved items remain"} below; retry sends only ${saveError.failedIndexes.length === 1 ? "it" : "them"}.`;
        setError(`${savedLabel} ${failedLabel}`);
      } else {
        setError(`Your ${scanType} was analyzed but not saved. Try saving again.`);
      }
    } finally {
      setIsSaving(false);
    }
  };

  const updateTextItem = (
    index: number,
    key: "name" | "category" | "expiryDate",
    value: string,
  ) => {
    setAnalysisResult((current) => {
      if (!current) return current;
      return current.map((item, itemIndex) =>
        itemIndex === index
          ? {
              ...item,
              [key]: value,
              ...(key === "expiryDate" ? { expiryIsEstimated: false } : {}),
            }
          : item
      );
    });
  };

  const updateStorageItem = (index: number, storage: FoodStorage) => {
    setAnalysisResult((current) => {
      if (!current) return current;
      return current.map((item, itemIndex) =>
        itemIndex === index ? { ...item, storage } : item
      );
    });
  };

  const updateNumericItem = (index: number, key: NumericItemKey, rawValue: string) => {
    const value = Number(rawValue);
    setAnalysisResult((current) => {
      if (!current) return current;
      return current.map((item, itemIndex) =>
        itemIndex === index
          ? { ...item, [key]: Number.isFinite(value) ? Math.max(0, value) : 0 }
          : item
      );
    });
  };

  const removeItem = (index: number) => {
    setAnalysisResult((current) => current?.filter((_, itemIndex) => itemIndex !== index) ?? null);
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Tab" || !dialogRef.current) return;
    const focusable = Array.from(
      dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)
    ).filter((element) => element.offsetParent !== null);
    if (focusable.length === 0) return;

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;
    if (event.shiftKey && (active === first || active === dialogRef.current)) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const switchMode = (nextMode: InputMode) => {
    setMode(nextMode);
    setAnalysisResult(null);
    setError(null);
  };

  const switchScanType = (nextScanType: ScanType) => {
    if (nextScanType === scanType) return;
    setScanType(nextScanType);
    setAnalysisResult(null);
    setError(null);
  };

  const scanCopy = SCAN_TYPE_COPY[scanType];

  const phaseCopy: Record<Exclude<AnalysisPhase, "idle">, string> = {
    preparing: "Preparing photo…",
    uploading: `Nutri is checking your ${scanType}…`,
    analyzing: `Nutri is reading your ${scanType} description…`,
  };

  const hasInvalidReviewItem = analysisResult?.some(
    (item) => !item.name.trim() || item.quantityGrams <= 0 || !item.expiryDate,
  ) ?? false;

  const saveActionLabel = analysisResult
    ? scanType === "ingredient"
      ? `Add ${analysisResult.length} ${analysisResult.length === 1 ? "item" : "items"} to pantry`
      : `Add ${analysisResult.length} ${analysisResult.length === 1 ? "item" : "items"} to today & pantry`
    : "Save scanned items";

  return (
    <div
      className="fixed inset-0 z-50 flex items-end bg-neutral-900/55 backdrop-blur-sm sm:items-center sm:justify-center sm:p-4 dark:bg-black/70"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) handleClose();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={TITLE_ID}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        className="flex h-[100dvh] w-full flex-col overflow-hidden bg-bg pt-[env(safe-area-inset-top)] shadow-lg focus:outline-none sm:max-h-[92vh] sm:max-w-xl sm:rounded-card sm:pt-0"
      >
        <div className="mx-auto mt-2 h-1.5 w-10 rounded-full bg-neutral-400 sm:hidden" aria-hidden="true" />

        <header className="flex items-center gap-3 border-b border-divider px-4 py-3 sm:px-6 sm:py-4">
          {analysisResult ? (
            <button
              type="button"
              onClick={() => {
                setAnalysisResult(null);
                setError(null);
              }}
              className="grid h-11 w-11 shrink-0 place-items-center rounded-full text-neutral-700 transition-colors hover:bg-neutral-200"
              aria-label="Back to scanner"
            >
              <ArrowLeft size={21} aria-hidden="true" />
            </button>
          ) : (
            <div className="hidden h-11 w-11 sm:block" aria-hidden="true" />
          )}

          <div className="min-w-0 flex-1 text-center">
            <p className="kicker text-accent-700">{analysisResult ? "Review" : "Quick add"}</p>
            <h2 id={TITLE_ID} className="mt-0.5 text-[22px] text-ink">
              {analysisResult ? scanCopy.reviewTitle : scanType === "food" ? "Log Food" : `Scan ${scanType}`}
            </h2>
          </div>

          <button
            type="button"
            onClick={handleClose}
            className="grid h-11 w-11 shrink-0 place-items-center rounded-full text-neutral-700 transition-colors hover:bg-neutral-200"
            aria-label="Close"
          >
            <X size={22} strokeWidth={2.5} aria-hidden="true" />
          </button>
        </header>

        <div className="flex-1 overflow-y-auto overscroll-contain px-5 pb-[calc(1.25rem+env(safe-area-inset-bottom))] pt-5 sm:px-6 sm:pb-5">
          {!analysisResult ? (
            <div className="mx-auto flex w-full max-w-lg flex-col gap-5">
              <fieldset>
                <legend className="kicker text-accent-700">What are you scanning?</legend>
                <div
                  className="mt-2 grid grid-cols-3 rounded-[20px] bg-neutral-200 p-1"
                  aria-describedby="scan-type-behavior"
                >
                  {SCAN_TYPE_OPTIONS.map(({ value, label, icon: Icon }) => (
                    <button
                      key={value}
                      type="button"
                      aria-pressed={scanType === value}
                      aria-describedby="scan-type-behavior"
                      onClick={() => switchScanType(value)}
                      className={`flex min-h-[52px] flex-col items-center justify-center gap-1 rounded-2xl px-2 text-xs transition-all sm:flex-row sm:text-sm ${
                        scanType === value ? "bg-surface font-semibold text-ink shadow-sm" : "text-neutral-700"
                      }`}
                    >
                      <Icon size={16} aria-hidden="true" /> {label}
                    </button>
                  ))}
                </div>
                <p
                  id="scan-type-behavior"
                  aria-live="polite"
                  className="mt-2.5 text-[13px] leading-relaxed text-neutral-700"
                >
                  {scanCopy.behavior}
                </p>
              </fieldset>

              <div
                role="group"
                aria-label={`How to scan ${scanType}`}
                className="grid grid-cols-2 rounded-full bg-neutral-200 p-1"
              >
                <button
                  type="button"
                  aria-pressed={mode === "image"}
                  onClick={() => switchMode("image")}
                  className={`flex min-h-[44px] items-center justify-center gap-2 rounded-full px-4 text-sm transition-all ${
                    mode === "image" ? "bg-surface font-semibold text-ink shadow-sm" : "text-neutral-700"
                  }`}
                >
                  <Camera size={17} aria-hidden="true" /> Photo
                </button>
                <button
                  type="button"
                  aria-pressed={mode === "text"}
                  onClick={() => switchMode("text")}
                  className={`flex min-h-[44px] items-center justify-center gap-2 rounded-full px-4 text-sm transition-all ${
                    mode === "text" ? "bg-surface font-semibold text-ink shadow-sm" : "text-neutral-700"
                  }`}
                >
                  <Type size={17} aria-hidden="true" /> Describe
                </button>
              </div>

              {mode === "image" ? (
                <div className="flex flex-col gap-4">
                  {previewUrl && selectedFile ? (
                    <>
                      <div className="relative aspect-[4/5] max-h-[52vh] overflow-hidden rounded-[28px] bg-neutral-200 shadow-sm sm:aspect-[4/3]">
                        <img
                          src={previewUrl}
                          alt={`Preview of the selected ${scanType === "food" ? "meal" : scanType}`}
                          className="h-full w-full object-cover"
                        />
                        <div className="absolute inset-x-3 bottom-3 flex items-center justify-between gap-2 rounded-full bg-neutral-900/75 px-3 py-2 text-xs text-white backdrop-blur-md">
                          <span className="min-w-0 truncate">{selectedFile.name}</span>
                          <span className="shrink-0 font-semibold">{formatFileSize(selectedFile.size)}</span>
                        </div>
                      </div>

                      <div className="grid grid-cols-2 gap-2.5">
                        <button
                          type="button"
                          onClick={() => cameraInputRef.current?.click()}
                          className="btn btn-secondary px-3"
                        >
                          <RotateCcw size={17} aria-hidden="true" /> Retake
                        </button>
                        <button
                          type="button"
                          onClick={() => galleryInputRef.current?.click()}
                          className="btn btn-secondary px-3"
                        >
                          <ImageIcon size={17} aria-hidden="true" /> Change photo
                        </button>
                      </div>
                    </>
                  ) : (
                    <>
                      <div className="rounded-[28px] bg-surface p-5 text-center shadow-sm">
                        <div className="mx-auto grid h-20 w-20 place-items-center rounded-full bg-accent-2-200 text-accent-2-800">
                          <Camera size={34} strokeWidth={2.25} aria-hidden="true" />
                        </div>
                        <h3 className="mt-4 text-[22px] text-ink">{scanCopy.photoHeading}</h3>
                        <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-neutral-600">
                          {scanCopy.photoHint}
                        </p>
                      </div>

                      <button
                        type="button"
                        onClick={() => cameraInputRef.current?.click()}
                        className="btn btn-primary w-full py-3.5 text-[15px] shadow-md"
                      >
                        <Camera size={20} strokeWidth={2.5} aria-hidden="true" /> Take a photo
                      </button>
                      <button
                        type="button"
                        onClick={() => galleryInputRef.current?.click()}
                        className="btn btn-secondary w-full py-3.5 text-[15px]"
                      >
                        <ImageIcon size={20} strokeWidth={2.5} aria-hidden="true" /> Choose from gallery
                      </button>
                    </>
                  )}

                  <input
                    ref={cameraInputRef}
                    type="file"
                    accept="image/jpeg,image/png,image/webp,image/heic,image/heif"
                    capture="environment"
                    className="hidden"
                    tabIndex={-1}
                    aria-hidden="true"
                    onChange={handleFileSelect}
                  />
                  <input
                    ref={galleryInputRef}
                    type="file"
                    accept="image/jpeg,image/png,image/webp,image/heic,image/heif"
                    className="hidden"
                    tabIndex={-1}
                    aria-hidden="true"
                    onChange={handleFileSelect}
                  />
                </div>
              ) : (
                <div>
                  <label className="field-label" htmlFor={TEXT_INPUT_ID}>{scanCopy.textLabel}</label>
                  <textarea
                    id={TEXT_INPUT_ID}
                    className="input h-44 resize-none rounded-[24px] px-4 py-3 text-base"
                    placeholder={scanCopy.textPlaceholder}
                    value={inputText}
                    onChange={(event) => setInputText(event.target.value)}
                  />
                </div>
              )}

              <p role="status" aria-live="polite" className={isAnalyzing ? "rounded-2xl bg-accent-2-100 px-4 py-3 text-sm font-semibold text-accent-2-900" : "sr-only"}>
                {isAnalyzing ? phaseCopy[analysisPhase] : ""}
              </p>
              {error && (
                <p role="alert" className="rounded-2xl bg-accent-100 px-4 py-3 text-sm text-accent-900">
                  {error}
                </p>
              )}

              {(mode === "text" || selectedFile) && (
                <button
                  type="button"
                  disabled={isAnalyzing || (mode === "text" ? !inputText.trim() : !selectedFile)}
                  onClick={handleAnalyze}
                  className="btn btn-primary sticky bottom-0 w-full py-3.5 text-[15px] shadow-md"
                >
                  {isAnalyzing ? (
                    <>
                      <Loader2 className="animate-spin" size={19} aria-hidden="true" />
                      {phaseCopy[analysisPhase]}
                    </>
                  ) : (
                    <>
                      <Sparkles size={19} aria-hidden="true" />
                      {mode === "image" ? "Use this photo" : "Analyze description"}
                    </>
                  )}
                </button>
              )}

              <div className="flex items-start gap-2.5 rounded-2xl border border-divider px-3.5 py-3 text-xs leading-relaxed text-neutral-600">
                <ShieldCheck size={17} className="mt-0.5 shrink-0 text-accent-2-700" aria-hidden="true" />
                <p>Photos are resized on this device and sent only for analysis. Quantity, expiry, storage, and nutrition can be edited before saving.</p>
              </div>
            </div>
          ) : (
            <div className="mx-auto flex w-full max-w-lg flex-col gap-4">
              <div className="rounded-[24px] bg-accent-2-100 px-4 py-3">
                <p className="flex items-center gap-2 text-sm font-semibold text-accent-2-900">
                  <Sparkles size={17} aria-hidden="true" /> Nutri found {analysisResult.length} {analysisResult.length === 1 ? "item" : "items"}
                </p>
                <p className="mt-1 text-xs leading-relaxed text-accent-2-900/80">
                  {scanCopy.behavior} AI estimates can be imperfect, so check every field before saving.
                </p>
              </div>

              <p role="status" aria-live="polite" className="sr-only">
                Analysis finished. Review {analysisResult.length} {analysisResult.length === 1 ? "item" : "items"} before saving.
              </p>

              <div className="flex flex-col gap-3">
                {analysisResult.map((item, index) => (
                  <fieldset key={index} className="rounded-[24px] bg-surface p-4 shadow-sm">
                    <legend className="sr-only">Detected item {index + 1}</legend>
                    <div className="flex items-start gap-2">
                      <div className="min-w-0 flex-1">
                        <label htmlFor={`food-name-${index}`} className="field-label">Food name</label>
                        <input
                          id={`food-name-${index}`}
                          value={item.name}
                          onChange={(event) => updateTextItem(index, "name", event.target.value)}
                          className="input bg-bg text-base font-semibold"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => removeItem(index)}
                        className="mt-[23px] grid h-11 w-11 shrink-0 place-items-center rounded-full text-neutral-700 transition-colors hover:bg-accent-100 hover:text-accent-800"
                        aria-label={`Remove ${item.name || `item ${index + 1}`}`}
                      >
                        <Trash2 size={18} aria-hidden="true" />
                      </button>
                    </div>

                    <div className="mt-3 grid grid-cols-1 gap-2.5 sm:grid-cols-2">
                      <label>
                        <span className="field-label">Quantity (g)</span>
                        <input
                          type="number"
                          min="0.01"
                          step="0.01"
                          value={item.quantityGrams}
                          onChange={(event) => updateNumericItem(index, "quantityGrams", event.target.value)}
                          className="input bg-bg tabular-nums"
                          aria-label={`Quantity (g) for ${item.name || `item ${index + 1}`}`}
                          required
                        />
                      </label>
                      <label>
                        <span className="field-label">
                          Expiration date{item.expiryIsEstimated ? " (estimated)" : ""}
                        </span>
                        <input
                          type="date"
                          value={item.expiryDate}
                          onChange={(event) => updateTextItem(index, "expiryDate", event.target.value)}
                          className="input bg-bg tabular-nums"
                          aria-label={`Expiration date for ${item.name || `item ${index + 1}`}`}
                          required
                        />
                      </label>
                      <label>
                        <span className="field-label">Storage</span>
                        <select
                          value={item.storage}
                          onChange={(event) => updateStorageItem(index, event.target.value as FoodStorage)}
                          className="input bg-bg"
                          aria-label={`Storage for ${item.name || `item ${index + 1}`}`}
                        >
                          <option value="fridge">Fridge</option>
                          <option value="freezer">Freezer</option>
                          <option value="pantry">Pantry</option>
                          <option value="other">Other</option>
                        </select>
                      </label>
                      <label>
                        <span className="field-label">Category</span>
                        <input
                          value={item.category}
                          onChange={(event) => updateTextItem(index, "category", event.target.value)}
                          className="input bg-bg"
                          aria-label={`Category for ${item.name || `item ${index + 1}`}`}
                        />
                      </label>
                    </div>

                    <p className="mt-4 text-[11px] font-semibold uppercase tracking-[0.08em] text-neutral-600">
                      Nutrition for the scanned quantity
                    </p>
                    <div className="mt-3 grid grid-cols-2 gap-2.5 sm:grid-cols-4">
                      {NUTRIENT_FIELDS.map(({ key, label, unit }) => (
                        <label key={key} className="rounded-2xl bg-bg px-3 py-2.5">
                          <span className="block text-[11px] font-semibold text-neutral-600">{label}</span>
                          <span className="mt-1 flex items-baseline gap-1">
                            <input
                              type="number"
                              min="0"
                              step={key === "calories" ? "1" : "0.1"}
                              value={item[key]}
                              onChange={(event) => updateNumericItem(index, key, event.target.value)}
                              className="min-w-0 flex-1 bg-transparent text-base font-semibold text-ink outline-none tabular-nums"
                              aria-label={`${label} for ${item.name || `item ${index + 1}`}`}
                            />
                            <span className="text-[11px] text-neutral-600">{unit}</span>
                          </span>
                        </label>
                      ))}
                    </div>
                  </fieldset>
                ))}
              </div>

              <button
                type="button"
                onClick={() => setAnalysisResult((current) => [...(current ?? []), emptyItem(scanType)])}
                className="btn btn-secondary w-full border-dashed"
              >
                <Plus size={17} aria-hidden="true" /> Add a missed item
              </button>

              {analysisResult.length === 0 && (
                <p className="rounded-2xl bg-accent-100 px-4 py-3 text-sm text-accent-900">
                  Add an item or go back and try another photo.
                </p>
              )}
              {error && (
                <p role="alert" className="rounded-2xl bg-accent-100 px-4 py-3 text-sm text-accent-900">
                  {error}
                </p>
              )}

              <button
                type="button"
                onClick={handleConfirm}
                disabled={isSaving || analysisResult.length === 0 || hasInvalidReviewItem}
                className="btn btn-primary sticky bottom-0 w-full py-3.5 text-[15px] shadow-md"
              >
                {isSaving ? <Loader2 className="animate-spin" size={19} aria-hidden="true" /> : <Check size={19} strokeWidth={2.75} aria-hidden="true" />}
                {isSaving
                  ? scanType === "ingredient" ? "Adding to pantry…" : "Saving to today & pantry…"
                  : saveActionLabel}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default AddEntryModal;
