// Domain types for the larder view (design 1c "Freshness Field") — the
// food-loss angle: what's in the kitchen, plotted by days until it turns.

export interface LarderItem {
  id: string;
  name: string;
  /** Short lowercase label under the timeline bubble ("spinach", "milk"). */
  label: string;
  /** Two-letter monogram inside the timeline bubble. */
  monogram: string;
  /** Days until the item turns; drives position, size and urgency color. */
  daysLeft: number;
}

// Demo larder. Items live in seed data for now; persistence through the
// backend is a follow-up — same treatment as food entries in App.
export const SEED_LARDER: LarderItem[] = [
  { id: "spinach", name: "Baby spinach", label: "spinach", monogram: "Sp", daysLeft: 1 },
  { id: "yogurt", name: "Greek yogurt", label: "yogurt", monogram: "Yo", daysLeft: 2 },
  { id: "raspberries", name: "Raspberries", label: "berries", monogram: "Ra", daysLeft: 2 },
  { id: "chicken", name: "Chicken breast", label: "chicken", monogram: "Ch", daysLeft: 3 },
  { id: "milk", name: "Milk", label: "milk", monogram: "Mi", daysLeft: 5 },
  { id: "carrots", name: "Carrots", label: "carrots", monogram: "Ca", daysLeft: 9 },
  { id: "eggs", name: "Eggs", label: "eggs", monogram: "Eg", daysLeft: 12 },
  { id: "rice", name: "Basmati rice", label: "rice", monogram: "Ri", daysLeft: 32 },
];

/** Items at daysLeft ≤ this feed the "eat soon" queue and its recipe card. */
export const EAT_SOON_DAYS = 2;

/** Days since anything in the larder was thrown away (demo aggregate). */
export const ZERO_WASTE_STREAK_DAYS = 11;
