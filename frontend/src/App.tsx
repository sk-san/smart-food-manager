import { Fragment, lazy, Suspense, useState, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { Camera, Carrot, LayoutGrid, Leaf, Loader2, Plus, RefreshCw, TrendingUp, User, LogOut } from "lucide-react";
import AddEntryModal from "./components/AddEntryModal";
import CompanionCharacter from "./components/CompanionCharacter";
import DashboardView, { WEEK_DAYS } from "./components/DashboardView";
import MobileDashboardView from "./components/MobileDashboardView";
import LoginView from "./components/LoginView";
import PantryView from "./components/PantryView";
import SettingsView from "./components/SettingsView";
import {
  FoodEntry,
  DailyGoal,
  SUGGESTED_GOALS,
  PENDING_PROFILE,
  GUEST_PROFILE,
  NutritionData,
  PartialScanSaveError,
  ScannedFoodInput,
  UserProfile,
} from "./types/nutrition";
import {
  DashboardLayout,
  readDashboardLayout,
  writeDashboardLayout,
} from "./preferences";
import { logNavigation, logScreenView } from "./telemetry/events";
import { ApiError, getToken, setToken } from "./api/client";
import { getCurrentUser, updateDisplayName } from "./services/accountService";
import { LarderItem, NewLarderItem, SEED_LARDER, WasteEvent } from "./types/pantry";
import {
  consumeInventoryItem,
  createInventoryItem,
  createScannedInventory,
  createWasteEvent,
  getGoals,
  listInventory,
  listMeals,
  listWasteEvents,
  putGoals,
  scanExpiryIsEstimated,
  scanNutritionPer100g,
} from "./services/persistenceService";

type Tab = "dashboard" | "history" | "pantry" | "settings";
type AuthState = "checking" | "authenticated" | "unauthenticated" | "error";

// Recharts is only needed on the Stats tab. Loading that screen on demand
// keeps the camera-first Android home bundle small and quick to resume.
const StatsView = lazy(() => import("./components/StatsView"));

const INITIAL_ENTRIES: FoodEntry[] = [];

const scaleNutrition = (nutrition: NutritionData, quantityGrams: number): NutritionData => {
  const scale = quantityGrams / 100;
  return {
    calories: nutrition.calories * scale,
    protein: nutrition.protein * scale,
    carbs: nutrition.carbs * scale,
    fat: nutrition.fat * scale,
    sodium: nutrition.sodium * scale,
    calcium: nutrition.calcium * scale,
    iron: nutrition.iron * scale,
  };
};

const upsertEntries = (
  current: FoodEntry[],
  incoming: FoodEntry[],
  deletedIds: string[] = [],
) => {
  const deleted = new Set(deletedIds);
  const next = current.filter((entry) => !deleted.has(entry.id));
  for (const entry of incoming) {
    const index = next.findIndex((candidate) => candidate.id === entry.id);
    if (index === -1) next.push(entry);
    else next[index] = entry;
  }
  return next;
};

const localDateKey = (date = new Date()) => {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
};

const localDaysUntil = (date?: string) => {
  if (!date) return 30;
  const today = new Date();
  const [year, month, day] = date.split("-").map(Number);
  if (![year, month, day].every(Number.isFinite)) return 30;
  const todayOrdinal = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  const expiryOrdinal = Date.UTC(year, month - 1, day);
  // Calendar ordinals avoid the 23/25-hour days around daylight-saving
  // changes. As on the backend, zero means the item remains through its date.
  return Math.round((expiryOrdinal - todayOrdinal) / 86_400_000);
};

const monogramFor = (name: string) => {
  const words = name.trim().split(/\s+/);
  return words.length > 1
    ? `${words[0][0]}${words[1][0]}`.toUpperCase()
    : name.slice(0, 2).replace(/^./, (letter) => letter.toUpperCase());
};

const GUEST_IMPACT_FACTORS = {
  beef: { co2: 99.48, water: 15_400, terms: ["beef", "steak", "牛"] },
  lamb: { co2: 39.72, water: 10_400, terms: ["lamb", "mutton", "sheep", "羊"] },
  pork: { co2: 12.31, water: 6_000, terms: ["pork", "pig", "bacon", "ham", "豚"] },
  chicken: { co2: 9.87, water: 4_300, terms: ["chicken", "poultry", "turkey", "鶏"] },
  eggs: { co2: 4.67, water: 3_300, terms: ["egg", "卵"] },
  cheese: { co2: 23.88, water: 5_000, terms: ["cheese", "チーズ"] },
  milk: { co2: 3.15, water: 1_000, terms: ["milk", "dairy", "yogurt", "乳", "ヨーグルト"] },
  rice: { co2: 4.45, water: 2_500, terms: ["rice", "米", "ご飯"] },
  grains: { co2: 1.57, water: 1_800, terms: ["grain", "bread", "wheat", "pasta", "cereal", "パン", "小麦"] },
  vegetables: { co2: 0.53, water: 322, terms: ["vegetable", "spinach", "tomato", "potato", "carrot", "野菜"] },
  fruit: { co2: 0.86, water: 962, terms: ["fruit", "apple", "banana", "berry", "orange", "果物"] },
  meat: { co2: 20, water: 7_000, terms: ["meat", "肉"] },
  prepared: { co2: 3, water: 1_000, terms: ["prepared", "meal", "dish", "ready"] },
} as const;

const guestImpact = (item: LarderItem, quantity: number) => {
  const kilograms = quantity / 1000;
  const searchable = `${item.category || ""} ${item.name}`.toLowerCase();
  // Authenticated responses are canonical. Guest mode copies the backend's
  // ordered category/name matches and published-factor snapshot so the bypass
  // remains an honest preview of the same feature.
  const categoryFactor = Object.values(GUEST_IMPACT_FACTORS)
    .find((candidate) => candidate.terms.some((term) => searchable.includes(term)))
    ?? { co2: 2.5, water: 1_000 };
  const impactKgCO2e = kilograms * categoryFactor.co2;
  return {
    impactKgCO2e,
    virtualWaterL: kilograms * categoryFactor.water,
    // EPA 2024 ten-year-growth average: 60 kg CO2e per urban tree-year.
    treeEquivalents: impactKgCO2e / 60,
  };
};

const scanSavedToast = (items: ScannedFoodInput[]) => {
  if (items.length === 1) return "Scanned item saved.";
  return `${items.length} scanned items saved.`;
};

const TAB_LABELS: Record<Tab, string> = {
  dashboard: "Today",
  history: "Stats",
  pantry: "Pantry",
  settings: "Account",
};

function App() {
  const [entries, setEntries] = useState<FoodEntry[]>(INITIAL_ENTRIES);
  const [goals, setGoals] = useState<DailyGoal>(SUGGESTED_GOALS);
  const [pantryItems, setPantryItems] = useState<LarderItem[]>([]);
  const [wasteEvents, setWasteEvents] = useState<WasteEvent[]>([]);
  const [isDataLoading, setIsDataLoading] = useState(false);
  const [dataError, setDataError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<Tab>("dashboard");
  const [isLookingAtContent, setIsLookingAtContent] = useState(false);
  const [dashboardLayout, setDashboardLayout] = useState<DashboardLayout>(readDashboardLayout);

  const [authState, setAuthState] = useState<AuthState>(() => getToken() ? "checking" : "unauthenticated");
  const [authError, setAuthError] = useState<string | null>(null);

  const [isGuest, setIsGuest] = useState(false);

  const initialCalendarDay = useRef(localDateKey());
  const [calendarDay, setCalendarDay] = useState(initialCalendarDay.current);
  const [savedDataRefresh, setSavedDataRefresh] = useState(0);

  // Who the account page and the header monogram describe. Null until /me
  // answers, so nothing claims an identity the server has not confirmed.
  const [account, setAccount] = useState<UserProfile | null>(null);
  const profile = isGuest ? GUEST_PROFILE : account ?? PENDING_PROFILE;
  const [isDark, setIsDark] = useState(() => document.documentElement.classList.contains("dark"));

  // Where the reader had each tab scrolled to. Without this, switching tabs
  // keeps the window offset from the tab you left, so a short screen can open
  // halfway down — or past its own end. A tab not yet visited restores to 0.
  const scrollOffsets = useRef<Partial<Record<Tab, number>>>({});
  const toastTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);
  const savedDataRequest = useRef(0);
  const lifecycleMutationRevision = useRef(0);
  const locallyExpiredItemIds = useRef(new Set<string>());

  useEffect(() => () => {
    if (toastTimeout.current) clearTimeout(toastTimeout.current);
  }, []);

  useEffect(() => {
    let midnightTimeout: ReturnType<typeof setTimeout> | null = null;

    const syncCalendarDay = () => {
      const nextDay = localDateKey();
      if (initialCalendarDay.current !== nextDay) {
        initialCalendarDay.current = nextDay;
        setCalendarDay(nextDay);
      }
    };
    const refreshSavedData = () => setSavedDataRefresh((current) => current + 1);
    const scheduleMidnight = () => {
      const now = new Date();
      const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
      midnightTimeout = setTimeout(() => {
        syncCalendarDay();
        refreshSavedData();
        scheduleMidnight();
      }, Math.max(100, tomorrow.getTime() - now.getTime() + 50));
    };
    const handleFocus = () => {
      syncCalendarDay();
      // A suspended tab may miss its timer, and the server owns the canonical
      // expiry date for authenticated stock. Refresh on every real resume so
      // a failed or timezone-early midnight check gets another chance.
      refreshSavedData();
    };
    const handleVisibilityChange = () => {
      if (document.visibilityState === "visible") handleFocus();
    };

    scheduleMidnight();
    window.addEventListener("focus", handleFocus);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      if (midnightTimeout) clearTimeout(midnightTimeout);
      window.removeEventListener("focus", handleFocus);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, []);

  const showToast = (message: string) => {
    if (toastTimeout.current) clearTimeout(toastTimeout.current);
    setToast(message);
    toastTimeout.current = setTimeout(() => setToast(null), 3_500);
  };

  useLayoutEffect(() => {
    window.scrollTo(0, scrollOffsets.current[activeTab] ?? 0);
  }, [activeTab]);

  const handleLogout = () => {
    logNavigation(activeTab, "login");
    savedDataRequest.current += 1;
    lifecycleMutationRevision.current += 1;
    setToken(null);
    setAuthState("unauthenticated");
    setAuthError(null);
    setIsGuest(false);
    setAccount(null);
    setEntries(INITIAL_ENTRIES);
    setGoals(SUGGESTED_GOALS);
    setPantryItems([]);
    setWasteEvents([]);
    locallyExpiredItemIds.current.clear();
    setDataError(null);
    setIsDataLoading(false);
    logScreenView("login");
  };

  const handleToggleDark = () => {
    setIsDark((prev) => {
      const next = !prev;
      document.documentElement.classList.toggle("dark", next);
      localStorage.setItem("nutrimind-theme", next ? "dark" : "light");
      return next;
    });
  };

  useEffect(() => {
    logScreenView("dashboard");
  }, []);

  // A persisted token is only a session candidate. Verify it with the server
  // before rendering authenticated UI: localStorage can contain an expired,
  // malformed, or previously signed token that the backend will reject.
  useEffect(() => {
    if (authState !== "checking") return;
    if (!getToken()) {
      setAuthState("unauthenticated");
      return;
    }

    let cancelled = false;
    setAuthError(null);
    getCurrentUser()
      .then((loaded) => {
        if (cancelled) return;
        setAccount(loaded);
        setAuthState("authenticated");
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        if (error instanceof ApiError && error.status === 401) {
          setToken(null);
          setAuthState("unauthenticated");
          return;
        }
        // A network outage or server error does not prove that the token is
        // invalid. Preserve it and give the user an explicit retry path.
        setAuthError("We couldn't verify your session. Check your connection and try again.");
        setAuthState("error");
      });

    return () => {
      cancelled = true;
    };
  }, [authState]);

  // Signing in through the form reaches "authenticated" without passing the
  // token check above, so the identity is loaded here instead. The account is
  // chrome rather than data, so a failure leaves the placeholder standing
  // rather than blocking the app behind an error.
  useEffect(() => {
    if (authState !== "authenticated" || isGuest || account || !getToken()) return;

    let cancelled = false;
    getCurrentUser()
      .then((loaded) => {
        if (!cancelled) setAccount(loaded);
      })
      .catch(() => undefined);

    return () => {
      cancelled = true;
    };
  }, [authState, isGuest, account]);

  useEffect(() => {
    if (authState !== "authenticated" || isGuest || !getToken()) return;

    let cancelled = false;
    const requestId = ++savedDataRequest.current;
    const mutationRevision = lifecycleMutationRevision.current;
    setIsDataLoading(true);
    setDataError(null);
    const loadSavedData = async () => {
      // Listing inventory performs server-side expiry reconciliation. Meals
      // and waste must be read afterwards so all three screens describe the
      // same post-reconciliation state.
      const lifecyclePromise = (async () => {
        const inventory = await listInventory();
        const [savedMeals, savedWaste] = await Promise.all([listMeals(), listWasteEvents()]);
        return { inventory, savedMeals, savedWaste };
      })();
      const [lifecycleResult, goalsResult] = await Promise.allSettled([
        lifecyclePromise,
        getGoals(),
      ]);
      if (cancelled || requestId !== savedDataRequest.current) return;

      // A pantry mutation that began while this snapshot was loading wins over
      // the older snapshot; otherwise a slow refresh could undo a consumption.
      const lifecycleIsCurrent = mutationRevision === lifecycleMutationRevision.current;
      if (lifecycleResult.status === "fulfilled" && lifecycleIsCurrent) {
        setPantryItems(lifecycleResult.value.inventory);
        setEntries(lifecycleResult.value.savedMeals);
        setWasteEvents(lifecycleResult.value.savedWaste);
      }
      if (goalsResult.status === "fulfilled") setGoals(goalsResult.value);
      if (
        (lifecycleResult.status === "rejected" && lifecycleIsCurrent) ||
        goalsResult.status === "rejected"
      ) {
        setDataError("Some saved data could not be loaded. Changes may not persist until the connection recovers.");
      } else if (lifecycleIsCurrent) {
        setDataError(null);
      }
    };

    void loadSavedData().finally(() => {
      if (!cancelled && requestId === savedDataRequest.current) setIsDataLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [authState, isGuest, calendarDay, savedDataRefresh]);

  useEffect(() => {
    if (!isGuest) return;

    const expired: LarderItem[] = [];
    let pantryChanged = false;
    const active = pantryItems.flatMap((item) => {
      if (!item.expiryDate) return [item];
      const daysLeft = localDaysUntil(item.expiryDate);
      if (daysLeft < 0) {
        pantryChanged = true;
        if (!locallyExpiredItemIds.current.has(item.id)) {
          locallyExpiredItemIds.current.add(item.id);
          expired.push(item);
        }
        return [];
      }
      if (daysLeft !== item.daysLeft) {
        pantryChanged = true;
        return [{ ...item, daysLeft }];
      }
      return [item];
    });

    if (pantryChanged) setPantryItems(active);
    if (expired.length === 0) return;

    const provisionalMealIds = new Set(
      expired.flatMap((item) => item.provisionalMealId ? [item.provisionalMealId] : []),
    );
    if (provisionalMealIds.size > 0) {
      // Expiry discards all stock still available, so any scan-time nutrition
      // assumption disappears just as it does in the authenticated lifecycle.
      setEntries((current) => current.filter((entry) => !provisionalMealIds.has(entry.id)));
    }

    const wastedAt = Date.now();
    const expiryWaste = expired.flatMap((item): WasteEvent[] => {
      const remaining = Math.max(
        0,
        (item.quantityPurchased ?? 0) -
          (item.quantityConsumed ?? 0) -
          (item.quantityWasted ?? 0),
      );
      if (remaining === 0) return [];
      return [{
        id: crypto.randomUUID(),
        inventoryItemId: item.id,
        foodName: item.name,
        quantity: remaining,
        wastedAt,
        reason: item.dateLabel === "use_by" ? "expired_use_by" : "expired_best_before",
        category: item.category,
        ...guestImpact(item, remaining),
      }];
    });
    if (expiryWaste.length > 0) {
      setWasteEvents((current) => [...expiryWaste, ...current]);
    }
  }, [calendarDay, isGuest, pantryItems]);

  const handleTabChange = (next: Tab) => {
    if (next === activeTab) return;
    scrollOffsets.current[activeTab] = window.scrollY;
    logNavigation(activeTab, next);
    setActiveTab(next);
    logScreenView(next);
  };

  const handleDashboardLayoutChange = (next: DashboardLayout) => {
    if (next === dashboardLayout) return;
    logNavigation(`dashboard:${dashboardLayout}`, `dashboard:${next}`);
    setDashboardLayout(next);
    writeDashboardLayout(next);
  };

  const todayEntries = useMemo(() => {
    const today = new Date();
    const start = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
    const end = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1).getTime();
    return entries.filter((entry) => entry.timestamp >= start && entry.timestamp < end);
  }, [calendarDay, entries]);

  // Real kcal per day for the running week, oldest first and today last, so
  // the dashboard's week chart plots actual meals for a signed-in account.
  // Bucketed by local calendar day, the same way todayEntries is, and taken
  // from `entries` because the meals endpoint returns the whole history.
  const weekCalories = useMemo(() => {
    const now = new Date();
    return Array.from({ length: WEEK_DAYS }, (_, i) => {
      const offset = WEEK_DAYS - 1 - i;
      const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - offset).getTime();
      const end = new Date(now.getFullYear(), now.getMonth(), now.getDate() - offset + 1).getTime();
      return entries
        .filter((entry) => entry.timestamp >= start && entry.timestamp < end)
        .reduce((sum, entry) => sum + entry.calories, 0);
    });
  }, [calendarDay, entries]);

  const todayTotals = useMemo(() => {
    return todayEntries.reduce(
      (acc, curr) => ({
        calories: acc.calories + curr.calories,
        protein: acc.protein + curr.protein,
        carbs: acc.carbs + curr.carbs,
        fat: acc.fat + curr.fat,
        sodium: acc.sodium + curr.sodium,
        calcium: acc.calcium + curr.calcium,
        iron: acc.iron + curr.iron,
      }),
      { calories: 0, protein: 0, carbs: 0, fat: 0, sodium: 0, calcium: 0, iron: 0 }
    );
  }, [todayEntries]);

  const handleAddEntries = async (newItems: ScannedFoodInput[]) => {
    if (isGuest || !getToken()) {
      const timestamp = Date.now();
      const pantry: LarderItem[] = [];
      const provisionalMeals: FoodEntry[] = [];
      for (const input of newItems) {
        const provisionalMealId = input.scanType === "ingredient" ? undefined : crypto.randomUUID();
        pantry.push({
          id: crypto.randomUUID(),
          name: input.name,
          label: input.name.split(/\s+/)[0].toLowerCase(),
          monogram: monogramFor(input.name),
          daysLeft: localDaysUntil(input.expiryDate),
          quantityPurchased: input.quantityGrams,
          quantityConsumed: 0,
          quantityWasted: 0,
          storage: input.storage,
          package: input.scanType === "food" ? "cooked" : "unopened",
          dateLabel: input.scanType === "product" ? "best_before" : "use_by",
          expiryDate: input.expiryDate,
          expiryIsEstimated: scanExpiryIsEstimated(input),
          sourceType: input.scanType,
          category: input.category || "other",
          provisionalMealId,
          nutritionPer100g: scanNutritionPer100g(input),
          isResolved: false,
        });
        if (provisionalMealId) {
          provisionalMeals.push({
            id: provisionalMealId,
            name: input.name,
            timestamp,
            calories: input.calories,
            protein: input.protein,
            carbs: input.carbs,
            fat: input.fat,
            sodium: input.sodium,
            calcium: input.calcium,
            iron: input.iron,
          });
        }
      }
      setPantryItems((current) => [...current, ...pantry]);
      setEntries((current) => upsertEntries(current, provisionalMeals));
      showToast(scanSavedToast(newItems));
      return;
    }

    lifecycleMutationRevision.current += 1;
    const results = await Promise.allSettled(newItems.map(createScannedInventory));
    // Invalidate snapshots that started while these writes were in flight.
    lifecycleMutationRevision.current += 1;
    const failed = results.find((result) => result.status === "rejected");
    if (failed) {
      const failedIndexes = results.flatMap((result, index) =>
        result.status === "rejected" ? [index] : [],
      );
      const succeededCount = results.length - failedIndexes.length;
      const succeeded = results.flatMap((result) =>
        result.status === "fulfilled" ? [result.value] : [],
      );
      // Wait for every scan request to finish before reconciling. Promise.all
      // would return on the first rejection while slower requests could still
      // commit after this snapshot, hiding successful items and inviting a
      // duplicate retry.
      try {
        const inventory = await listInventory();
        const [savedMeals, savedWaste] = await Promise.all([listMeals(), listWasteEvents()]);
        setPantryItems(inventory);
        setEntries(savedMeals);
        setWasteEvents(savedWaste);
      } catch {
        // Successful POST responses remain authoritative even if the
        // follow-up snapshot is unavailable. Merge them so committed items do
        // not disappear locally until the next focus/refresh.
        setPantryItems((current) => {
          const byId = new Map(current.map((item) => [item.id, item]));
          succeeded.forEach((result) => {
            if (!result.inventory.isResolved) byId.set(result.inventory.id, result.inventory);
          });
          return [...byId.values()];
        });
        setEntries((current) => upsertEntries(
          current,
          succeeded.flatMap((result) => result.meal ? [result.meal] : []),
        ));
      }
      setDataError(succeededCount > 0
        ? `${succeededCount} scanned ${succeededCount === 1 ? "item was" : "items were"} saved. Retry only the unsaved ${failedIndexes.length === 1 ? "item" : "items"} left in the scanner.`
        : "The scanned items could not be saved. Please try again.");
      if (succeededCount > 0) {
        throw new PartialScanSaveError(failedIndexes, succeededCount);
      }
      throw failed.reason;
    }

    const saved = results.flatMap((result) => result.status === "fulfilled" ? [result.value] : []);
    setPantryItems((current) => {
      const byId = new Map(current.map((item) => [item.id, item]));
      saved.forEach((result) => {
        if (!result.inventory.isResolved) byId.set(result.inventory.id, result.inventory);
      });
      return [...byId.values()];
    });
    setEntries((current) => upsertEntries(
      current,
      saved.flatMap((result) => result.meal ? [result.meal] : []),
    ));
    setDataError(null);
    showToast(scanSavedToast(newItems));
  };

  const handleUpdateDisplayName = async (name: string) => {
    setAccount(await updateDisplayName(name));
  };

  const handleUpdateGoals = async (next: DailyGoal) => {
    if (isGuest || !getToken()) {
      setGoals(next);
      return;
    }
    const saved = await putGoals(next);
    setGoals(saved);
    setDataError(null);
  };

  const handleAddPantryItem = async (input: NewLarderItem) => {
    if (isGuest || !getToken()) {
      const item: LarderItem = {
        id: crypto.randomUUID(),
        name: input.name,
        label: input.name.split(/\s+/)[0].toLowerCase(),
        monogram: monogramFor(input.name),
        daysLeft: localDaysUntil(input.expiryDate),
        quantityPurchased: input.quantityPurchased,
        quantityConsumed: 0,
        quantityWasted: 0,
        storage: input.storage,
        package: "unopened",
        dateLabel: input.expiryDate ? "best_before" : "unknown",
        expiryDate: input.expiryDate,
        expiryIsEstimated: false,
        sourceType: "ingredient",
        category: "other",
        isResolved: false,
      };
      setPantryItems((current) => [...current, item]);
      return;
    }
    lifecycleMutationRevision.current += 1;
    const saved = await createInventoryItem(input);
    lifecycleMutationRevision.current += 1;
    setPantryItems((current) => {
      const existing = current.findIndex((item) => item.id === saved.id);
      if (existing === -1) return [...current, saved];
      return current.map((item, index) => index === existing ? saved : item);
    });
    setDataError(null);
  };

  const handleWasteItem = async (item: LarderItem, quantity: number, reason: string) => {
    if (isGuest || !getToken()) {
      const impact = guestImpact(item, quantity);
      const wasted = (item.quantityWasted ?? 0) + quantity;
      if (item.provisionalMealId && item.nutritionPer100g) {
        const provisionalMealId = item.provisionalMealId;
        const nutritionPer100g = item.nutritionPer100g;
        const provisionalQuantity = Math.max(
          0,
          (item.quantityPurchased ?? 0) - (item.quantityConsumed ?? 0) - wasted,
        );
        const currentProvisional = entries.find((entry) => entry.id === provisionalMealId);
        setEntries((current) => upsertEntries(
          current,
          provisionalQuantity > 0
            ? [{
                id: provisionalMealId,
                name: item.name,
                timestamp: currentProvisional?.timestamp ?? Date.now(),
                ...scaleNutrition(nutritionPer100g, provisionalQuantity),
              }]
            : [],
          provisionalQuantity === 0 ? [provisionalMealId] : [],
        ));
      }
      setWasteEvents((current) => [{
        id: crypto.randomUUID(), inventoryItemId: item.id, foodName: item.name,
        quantity, wastedAt: Date.now(), reason, category: item.category, ...impact,
      }, ...current]);
    } else {
      lifecycleMutationRevision.current += 1;
      const saved = await createWasteEvent(item, quantity, reason);
      lifecycleMutationRevision.current += 1;
      setWasteEvents((current) => [saved, ...current.filter((event) => event.id !== saved.id)]);
      // Manual waste can resize or delete a provisional food/product meal.
      // The waste response intentionally stays focused on waste, so reload
      // the intake ledger after the mutation commits.
      try {
        setEntries(await listMeals());
        setDataError(null);
      } catch {
        setDataError("Waste was saved, but today's nutrition could not be refreshed. Reload to reconcile it.");
      }
    }
    setPantryItems((current) => current.flatMap((candidate) => {
      if (candidate.id !== item.id) return [candidate];
      const wasted = (candidate.quantityWasted ?? 0) + quantity;
      const resolved = (candidate.quantityConsumed ?? 0) + wasted >= (candidate.quantityPurchased ?? Infinity);
      return resolved ? [] : [{ ...candidate, quantityWasted: wasted }];
    }));
  };

  const handleConsumeItem = async (
    item: LarderItem,
    quantity: number,
    discardRemaining: boolean,
    wasteReason: string,
  ) => {
    if (isGuest || !getToken()) {
      const purchased = item.quantityPurchased ?? 0;
      const consumedBefore = item.quantityConsumed ?? 0;
      const wastedBefore = item.quantityWasted ?? 0;
      const remainingBefore = Math.max(0, purchased - consumedBefore - wastedBefore);
      const consumed = consumedBefore + quantity;
      const discarded = discardRemaining ? Math.max(0, remainingBefore - quantity) : 0;
      const wasted = wastedBefore + discarded;
      const resolved = consumed + wasted >= purchased;
      const nutrition = item.nutritionPer100g;
      let provisionalMealId = item.provisionalMealId;

      if (nutrition) {
        const preparedSource = item.sourceType === "food" || item.sourceType === "product";
        const incoming: FoodEntry[] = [];
        const deleted: string[] = [];
        if (preparedSource && provisionalMealId) {
          if (quantity > 0) {
            incoming.push({
              id: provisionalMealId,
              name: item.name,
              timestamp: entries.find((entry) => entry.id === provisionalMealId)?.timestamp ?? Date.now(),
              ...scaleNutrition(nutrition, quantity),
            });
          } else {
            deleted.push(provisionalMealId);
          }
          // The first consumption finalizes the provisional scan entry. Any
          // later portions are separate meals at their actual consumption time.
          provisionalMealId = undefined;
        } else if (quantity > 0) {
          incoming.push({
            id: crypto.randomUUID(),
            name: item.name,
            timestamp: Date.now(),
            ...scaleNutrition(nutrition, quantity),
          });
        }
        setEntries((current) => upsertEntries(current, incoming, deleted));
      }

      setPantryItems((current) => current.flatMap((candidate) => {
        if (candidate.id !== item.id) return [candidate];
        if (resolved) return [];
        return [{
          ...candidate,
          quantityConsumed: consumed,
          quantityWasted: wasted,
          provisionalMealId,
          isResolved: false,
        }];
      }));

      if (discarded > 0) {
        const impact = guestImpact(item, discarded);
        setWasteEvents((current) => [{
          id: crypto.randomUUID(),
          inventoryItemId: item.id,
          foodName: item.name,
          quantity: discarded,
          wastedAt: Date.now(),
          reason: wasteReason,
          category: item.category,
          ...impact,
        }, ...current]);
      }
      showToast(discarded > 0 ? "Consumption and waste saved." : "Consumption saved.");
      return;
    }

    lifecycleMutationRevision.current += 1;
    const result = await consumeInventoryItem(item, quantity, discardRemaining, wasteReason);
    lifecycleMutationRevision.current += 1;
    setPantryItems((current) => current.flatMap((candidate) => {
      if (candidate.id !== item.id) return [candidate];
      return result.inventory.isResolved ? [] : [result.inventory];
    }));
    setEntries((current) => upsertEntries(
      current,
      result.meal ? [result.meal] : [],
      result.deletedMealId ? [result.deletedMealId] : [],
    ));
    if (result.wasteEvent) {
      const savedWaste = result.wasteEvent;
      setWasteEvents((current) => [savedWaste, ...current.filter((event) => event.id !== savedWaste.id)]);
    }
    setDataError(null);
    showToast(result.wasteEvent ? "Consumption and waste saved." : "Consumption saved.");
  };

  const handleContentHoverStart = () => setIsLookingAtContent(true);
  const handleContentHoverEnd = () => setIsLookingAtContent(false);

  const enterApp = (from: string) => {
    setAuthState("authenticated");
    setAuthError(null);
    setActiveTab("dashboard");
    scrollOffsets.current = {};
    logNavigation(from, "dashboard");
    logScreenView("dashboard");
  };

  const handleSignIn = () => enterApp("login");

  const handleGuestLogin = () => {
    locallyExpiredItemIds.current.clear();
    setIsGuest(true);
    setPantryItems(SEED_LARDER);
    enterApp("login:guest");
  };

  const navItems: { tab: Tab; icon: typeof LayoutGrid }[] = [
    { tab: "dashboard", icon: LayoutGrid },
    { tab: "history", icon: TrendingUp },
    { tab: "pantry", icon: Carrot },
    { tab: "settings", icon: User },
  ];

  if (authState === "checking") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg p-6" role="status" aria-live="polite">
        <div className="flex flex-col items-center gap-3 text-center text-ink">
          <Loader2 className="animate-spin text-accent-700" size={30} aria-hidden="true" />
          <p className="font-display text-xl">Checking your session…</p>
        </div>
      </div>
    );
  }

  if (authState === "error") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg p-6">
        <div className="w-full max-w-md rounded-card bg-surface p-8 text-center shadow-lg">
          <h1 className="text-2xl text-ink">We couldn't verify your session</h1>
          <p role="alert" className="mt-3 text-sm leading-relaxed text-neutral-700">{authError}</p>
          <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:justify-center">
            <button
              type="button"
              className="btn btn-primary justify-center"
              onClick={() => setAuthState("checking")}
            >
              <RefreshCw size={16} aria-hidden="true" />
              Retry
            </button>
            <button type="button" className="btn btn-secondary justify-center" onClick={handleLogout}>
              Sign in instead
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (authState === "unauthenticated") {
    return <LoginView onSignIn={handleSignIn} onGuestLogin={handleGuestLogin} />;
  }

  const brand = (
    <button
      onClick={() => handleTabChange("dashboard")}
      aria-label="Today"
      className="flex items-center gap-2.5"
    >
      <div className="grid h-9 w-9 place-items-center rounded-full bg-accent-2-300 transition-transform hover:scale-105 active:scale-95">
        <Leaf size={18} strokeWidth={2.75} className="text-accent-2-800" />
      </div>
      <span className="font-display text-[19px] text-ink">Nutri</span>
    </button>
  );

  const avatar = (
    <button
      onClick={() => handleTabChange("settings")}
      aria-label="Account"
      aria-current={activeTab === "settings" ? "page" : undefined}
      className="grid h-11 w-11 place-items-center rounded-full text-[13px] font-semibold text-accent-900 transition-transform hover:scale-105 active:scale-95"
    >
      <span className="grid h-9 w-9 place-items-center rounded-full bg-accent-300">{profile.initials}</span>
    </button>
  );

  return (
    <div className="flex min-h-screen flex-col bg-bg">
      {toast && (
        <div
          role="status"
          aria-live="polite"
          className="fixed left-1/2 top-[calc(1rem+env(safe-area-inset-top))] z-[60] -translate-x-1/2 rounded-full bg-ink px-4 py-2.5 text-sm font-semibold text-bg shadow-lg"
        >
          {toast}
        </div>
      )}
      {/* Top navigation (desktop) */}
      <header className="mx-auto hidden w-full max-w-6xl items-center gap-7 px-8 pt-7 md:flex">
        <div className="mr-auto">{brand}</div>
        <nav aria-label="Primary" className="flex items-center gap-6">
          {navItems.map(({ tab }) => (
            <button
              key={tab}
              onClick={() => handleTabChange(tab)}
              aria-current={activeTab === tab ? "page" : undefined}
              className={`text-[13px] transition-colors ${
                activeTab === tab
                  ? "font-semibold text-accent-700"
                  : "text-neutral-700 hover:text-ink"
              }`}
            >
              {TAB_LABELS[tab]}
            </button>
          ))}
        </nav>
        <button onClick={() => setIsModalOpen(true)} className="btn btn-primary shadow-sm">
          <Plus size={15} strokeWidth={2.75} />
          Log food
        </button>
        {avatar}
        <button
          onClick={handleLogout}
          className="ml-2 flex items-center gap-1 rounded-lg px-3 py-1.5 text-[13px] font-medium text-neutral-700 transition-all hover:bg-neutral-100 hover:text-accent-700"
        >
          <LogOut size={16} />
          <span>Log Out</span>
        </button>
      </header>

      {/* Compact top bar (mobile) */}
      <div className="flex items-center justify-between px-5 pt-[calc(1.25rem+env(safe-area-inset-top))] md:hidden">
        {brand}
        {avatar}
      </div>

      {/* Main content. The bottom padding clears the fixed tab bar plus the
          home indicator on notched phones. */}
      <main className="relative mx-auto w-full max-w-6xl flex-1 px-5 pb-[calc(7rem+env(safe-area-inset-bottom))] pt-5 md:px-8 md:pb-14 md:pt-6">
        {dataError && (
          <div role="alert" className="mb-4 rounded-2xl bg-accent-100 px-4 py-3 text-sm text-accent-900">
            {dataError}
          </div>
        )}
        {activeTab === "dashboard" ? (
          <>
            <div className="md:hidden">
              <MobileDashboardView
                todayTotals={todayTotals}
                goals={goals}
                entries={todayEntries}
                firstName={profile.name.split(/\s+/)[0]}
                onLogFood={() => setIsModalOpen(true)}
                onOpenStats={() => handleTabChange("history")}
              />
            </div>
            <div className="hidden md:block">
              <DashboardView
                todayTotals={todayTotals}
                goals={goals}
                entries={todayEntries}
                layout={dashboardLayout}
                isGuest={isGuest}
                weekCalories={weekCalories}
                onLogFood={() => setIsModalOpen(true)}
                onOpenStats={() => handleTabChange("history")}
                onHoverStart={handleContentHoverStart}
                onHoverEnd={handleContentHoverEnd}
              />
            </div>
          </>
        ) : activeTab === "history" ? (
          <Suspense
            fallback={(
              <div className="flex min-h-[40vh] items-center justify-center" role="status" aria-live="polite">
                <Loader2 className="animate-spin text-accent-700" size={28} aria-hidden="true" />
                <span className="sr-only">Loading statistics…</span>
              </div>
            )}
          >
            <StatsView entries={entries} goals={goals} />
          </Suspense>
        ) : activeTab === "pantry" ? (
          <PantryView
            items={pantryItems}
            wasteEvents={wasteEvents}
            isLoading={isDataLoading}
            onAddItem={handleAddPantryItem}
            onConsumeItem={handleConsumeItem}
            onWasteItem={handleWasteItem}
            onHoverStart={handleContentHoverStart}
            onHoverEnd={handleContentHoverEnd}
          />
        ) : (
          <SettingsView
            goals={goals}
            onUpdateGoals={handleUpdateGoals}
            isDark={isDark}
            onToggleDark={handleToggleDark}
            onSignOut={handleLogout}
            profile={profile}
            onUpdateDisplayName={isGuest ? undefined : handleUpdateDisplayName}
            dashboardLayout={dashboardLayout}
            onDashboardLayoutChange={handleDashboardLayoutChange}
          />
        )}
      </main>

      {/* Nutri — persistent across views, but stands down while the log-food
          dialog owns the screen. */}
      <div className="hidden md:block">
        <CompanionCharacter
          stats={todayTotals}
          goals={goals}
          isLookingAtScreen={isLookingAtContent}
          isSuppressed={isModalOpen}
        />
      </div>

      <AddEntryModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onAdd={handleAddEntries}
        isGuest={isGuest}
      />

      {/* Mobile tab bar */}
      <nav
        aria-label="Primary"
        className="pb-safe fixed inset-x-0 bottom-0 z-40 border-t border-divider bg-surface md:hidden"
      >
        <div className="grid h-20 grid-cols-5 items-center px-2">
          {navItems.map(({ tab, icon: Icon }, index) => (
            <Fragment key={tab}>
              {index === 2 && (
                <button
                  type="button"
                  onClick={() => setIsModalOpen(true)}
                  className="flex min-h-[62px] flex-col items-center justify-center gap-0.5 text-accent-700"
                  aria-label="Scan food"
                >
                  <span className="grid h-12 w-12 place-items-center rounded-full bg-accent-solid text-bg shadow-md">
                    <Camera size={22} strokeWidth={2.75} aria-hidden="true" />
                  </span>
                  <span className="text-[11px] font-semibold">Scan</span>
                </button>
              )}
              <button
                onClick={() => handleTabChange(tab)}
                aria-current={activeTab === tab ? "page" : undefined}
                className={`flex min-h-[52px] flex-col items-center justify-center gap-1 rounded-2xl px-1 transition-colors ${
                  activeTab === tab ? "text-accent-700" : "text-neutral-700"
                }`}
              >
                <Icon size={22} strokeWidth={2.75} aria-hidden="true" />
                <span className={`text-[11px] ${activeTab === tab ? "font-semibold" : "font-medium"}`}>
                  {TAB_LABELS[tab]}
                </span>
              </button>
            </Fragment>
          ))}
        </div>
      </nav>
    </div>
  );
}

export default App;
