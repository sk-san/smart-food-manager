// Per-browser display preferences. Nothing here affects what the app records —
// only how it is drawn — so localStorage is the whole story.

/**
 * The Today tab has three possible layouts, all reading the same entries and
 * goals: the Kitchen Ledger (1a), the Day Plate (1b) and the Almanac (1d) from
 * the Dashboard Ideas doc. They are alternative *presentations* of one task,
 * not three separate tasks, so Today shows exactly one — the Ledger unless the
 * reader has chosen otherwise — and the choice lives in Account beside the
 * other personalization. All three are offered at every width.
 */
export const DASHBOARD_LAYOUTS = [
  {
    id: "ledger",
    label: "Ledger",
    description: "Cards and gauges. The fullest account of the day.",
  },
  {
    id: "plate",
    label: "Plate",
    description: "A single clock face, with each meal set at the hour you ate it.",
  },
  {
    id: "almanac",
    label: "Almanac",
    description: "The day written out as a page, with the numbers in the margin.",
  },
] as const;

export type DashboardLayout = (typeof DASHBOARD_LAYOUTS)[number]["id"];

export const DEFAULT_DASHBOARD_LAYOUT: DashboardLayout = "ledger";

const LAYOUT_STORAGE_KEY = "nutri-dashboard-board";

const isLayout = (value: unknown): value is DashboardLayout =>
  DASHBOARD_LAYOUTS.some((layout) => layout.id === value);

export const readDashboardLayout = (): DashboardLayout => {
  try {
    const stored = localStorage.getItem(LAYOUT_STORAGE_KEY);
    return isLayout(stored) ? stored : DEFAULT_DASHBOARD_LAYOUT;
  } catch {
    // Private-mode Safari and friends throw on access rather than returning null.
    return DEFAULT_DASHBOARD_LAYOUT;
  }
};

export const writeDashboardLayout = (layout: DashboardLayout) => {
  try {
    localStorage.setItem(LAYOUT_STORAGE_KEY, layout);
  } catch {
    /* preference is best-effort */
  }
};

/**
 * Whether Nutri is folded away to a badge.
 *
 * The companion is fixed chrome laid over the content, so it only earns its
 * place where there is room beside the page for it. Null means the reader has
 * not expressed a preference, and the component falls back to
 * COMPANION_ROOM_QUERY — below that width Nutri starts folded away, because
 * anywhere narrower it would be sitting on top of a card.
 */
const COMPANION_STORAGE_KEY = "nutri-companion-collapsed";

/**
 * Content is max-w-6xl (1152px) plus 2×32px of padding = 1216px, and the cat
 * needs ~146px of clear gutter beside that.
 */
export const COMPANION_ROOM_QUERY = "(min-width: 1520px)";

export const readCompanionCollapsed = (): boolean | null => {
  try {
    const stored = localStorage.getItem(COMPANION_STORAGE_KEY);
    return stored === null ? null : stored === "true";
  } catch {
    return null;
  }
};

export const writeCompanionCollapsed = (collapsed: boolean) => {
  try {
    localStorage.setItem(COMPANION_STORAGE_KEY, String(collapsed));
  } catch {
    /* preference is best-effort */
  }
};
