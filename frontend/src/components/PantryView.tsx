import React, { useState } from 'react';
import { Droplets, LineChart, Loader2, Plus, Rows3, Trash2, Trees, Utensils } from 'lucide-react';
import PlannedAction from './PlannedAction';
import { useIsMobile } from '../hooks/useMediaQuery';
import { EAT_SOON_DAYS, LarderItem, NewLarderItem, WasteEvent } from '../types/pantry';

interface PantryViewProps {
  items: LarderItem[];
  wasteEvents: WasteEvent[];
  isLoading: boolean;
  onAddItem: (item: NewLarderItem) => void | Promise<void>;
  onConsumeItem: (
    item: LarderItem,
    quantity: number,
    discardRemaining: boolean,
    wasteReason: string,
  ) => void | Promise<void>;
  onWasteItem: (item: LarderItem, quantity: number, reason: string) => void | Promise<void>;
  onHoverStart: () => void;
  onHoverEnd: () => void;
}

// Ripeness timeline (design 1c): horizontal position is days until an item
// turns, on a log scale so the urgent left end gets the room.
const TIMELINE_MAX_DAYS = 30;
const timelineLeftPct = (daysLeft: number) => {
  const clamped = Math.min(Math.max(daysLeft, 1), TIMELINE_MAX_DAYS);
  return 6 + 83 * (Math.log(clamped) / Math.log(TIMELINE_MAX_DAYS));
};

// Timeline geometry. Bubbles share one baseline and labels alternate above and
// below it: the log scale crowds the urgent end, so neighbours can land close
// together, and two label lanes mean a tight cluster still reads.
const BUBBLE_BOTTOM = 74;
const LABEL_BOTTOM_BELOW = 38;
const LABEL_BOTTOM_ABOVE = 140;
/**
 * Floor on the horizontal gap between two bubbles, as a percentage of the
 * canvas. Enough that a bubble never sits on its neighbour; with the labels
 * alternating lanes it is also enough for the text.
 */
const MIN_GAP_PCT = 9.5;

const bubbleSize = (daysLeft: number) => (daysLeft <= 1 ? 56 : daysLeft <= 2 ? 48 : daysLeft <= 3 ? 52 : 46);

const urgencyBorder = (daysLeft: number) =>
  daysLeft <= 2
    ? 'var(--color-accent-500)'
    : daysLeft <= 4
      ? 'var(--color-accent-300)'
      : daysLeft <= 6
        ? 'var(--color-accent-2-400)'
        : daysLeft <= 14
          ? 'var(--color-accent-2-500)'
          : 'var(--color-accent-2-600)';

const daysLabel = (daysLeft: number) => (daysLeft >= TIMELINE_MAX_DAYS ? `${TIMELINE_MAX_DAYS}d+` : `${daysLeft}d`);

// The list groups the larder the way a cook reads it, so urgency is a heading
// rather than a colour the reader has to decode.
const BANDS = [
  { id: 'soon', title: 'Eat soon', caption: 'today or tomorrow', max: EAT_SOON_DAYS },
  { id: 'week', title: 'This week', caption: 'within seven days', max: 7 },
  { id: 'later', title: 'Keeps well', caption: 'more than a week left', max: Infinity },
] as const;

/** Calendar date an item turns, so "3 days" is also an actual day. */
const turnsOn = (daysLeft: number) => {
  const date = new Date();
  date.setDate(date.getDate() + daysLeft);
  return date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
};

const plainDaysLeft = (daysLeft: number) => {
  if (daysLeft <= 0) return 'turns today';
  if (daysLeft === 1) return '1 day left';
  if (daysLeft >= TIMELINE_MAX_DAYS) return `over ${TIMELINE_MAX_DAYS} days left`;
  return `${daysLeft} days left`;
};

/**
 * One drifting wave crest, drawn 120 units wide across an 80-unit viewBox so
 * there is always surplus to slide in from the right. The shape repeats every
 * 40 units, which is exactly what `water-drift` translates, so the loop has no
 * seam.
 */
const WAVE_PATH = 'M0 8 q10 -4 20 0 t20 0 t20 0 t20 0 t20 0 t20 0 V24 H0 Z';

/** Foods named individually before the rest are folded into one row. */
const NAMED_DRIVERS = 4;
/** Rank as tone: the heaviest driver is solid, the tail fades. */
const DRIVER_OPACITY = [1, 0.68, 0.46, 0.3, 0.2];

/**
 * Waste grouped by food and ranked by CO₂e rather than by mass, because the two
 * are not the same thing: 40 g of beef outweighs half a kilo of vegetables. The
 * category factors already encode that; until now nothing in the UI showed it.
 */
const impactDrivers = (events: WasteEvent[], totalCO2e: number) => {
  if (totalCO2e <= 0) return [];
  const byFood = new Map<string, { name: string; kgCO2e: number; quantity: number }>();
  events.forEach((event) => {
    const key = event.foodName.trim().toLowerCase();
    const existing = byFood.get(key);
    if (existing) {
      existing.kgCO2e += event.impactKgCO2e;
      existing.quantity += event.quantity;
      return;
    }
    byFood.set(key, {
      name: event.foodName,
      kgCO2e: event.impactKgCO2e,
      quantity: event.quantity,
    });
  });
  const ranked = [...byFood.values()].sort((a, b) => b.kgCO2e - a.kgCO2e);
  const tail = ranked.slice(NAMED_DRIVERS);
  const drivers = tail.length
    ? [
        ...ranked.slice(0, NAMED_DRIVERS),
        {
          name: `${tail.length} more foods`,
          kgCO2e: tail.reduce((total, driver) => total + driver.kgCO2e, 0),
          quantity: tail.reduce((total, driver) => total + driver.quantity, 0),
        },
      ]
    : ranked;
  return drivers.map((driver) => ({ ...driver, sharePct: (driver.kgCO2e / totalCO2e) * 100 }));
};

const LarderRow: React.FC<{
  item: LarderItem;
  onConsume: (item: LarderItem) => void;
  onWaste: (item: LarderItem) => void;
}> = ({ item, onConsume, onWaste }) => {
  const urgent = item.daysLeft <= EAT_SOON_DAYS;
  const remaining = (item.quantityPurchased ?? 0) - (item.quantityConsumed ?? 0) - (item.quantityWasted ?? 0);
  return (
    <li className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3.5 gap-y-2 py-3 sm:grid-cols-[auto_minmax(0,1fr)_auto]">
      <span
        className="grid h-11 w-11 flex-none place-items-center rounded-full bg-neutral-100 text-[15px] font-semibold text-ink"
        style={{ border: `2.5px solid ${urgencyBorder(item.daysLeft)}` }}
        aria-hidden="true"
      >
        {item.monogram}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[15px] font-semibold text-ink">{item.name}</span>
        {/* Everything the old timeline hid behind a hover tooltip, in text. */}
        <span className="block text-[13px] text-neutral-700 tabular-nums">
          {plainDaysLeft(item.daysLeft)} · turns {turnsOn(item.daysLeft)}
        </span>
        {item.quantityPurchased !== undefined && (
          <span className="block text-xs text-neutral-600 tabular-nums">
            {remaining.toLocaleString('en-US')} g remaining · {item.storage}
            {item.sourceType ? ` · ${item.sourceType}` : ''}
            {item.expiryIsEstimated ? ' · estimated expiry' : ''}
          </span>
        )}
      </span>
      <span className="col-span-2 flex min-w-0 items-center justify-end gap-1.5 sm:col-span-1">
        <span className={`tag flex-none tabular-nums ${urgent ? 'tag-accent font-semibold' : 'tag-neutral'}`}>
          {daysLabel(item.daysLeft)}
        </span>
        {item.quantityPurchased !== undefined && (
          <>
            <button
              type="button"
              onClick={() => onConsume(item)}
              aria-label={`Record consumption for ${item.name}`}
              className="grid h-10 w-10 flex-none place-items-center rounded-full text-neutral-600 transition-colors hover:bg-accent-2-200 hover:text-accent-2-900"
            >
              <Utensils size={16} strokeWidth={2.5} aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={() => onWaste(item)}
              aria-label={`Log waste for ${item.name}`}
              className="grid h-10 w-10 flex-none place-items-center rounded-full text-neutral-600 transition-colors hover:bg-accent-100 hover:text-accent-800"
            >
              <Trash2 size={16} strokeWidth={2.5} aria-hidden="true" />
            </button>
          </>
        )}
      </span>
    </li>
  );
};

const PantryView: React.FC<PantryViewProps> = ({
  items,
  wasteEvents,
  isLoading,
  onAddItem,
  onConsumeItem,
  onWasteItem,
  onHoverStart,
  onHoverEnd,
}) => {
  const isMobile = useIsMobile();
  // The timeline is a wide canvas by construction, so it is offered only where
  // there is width for it. On a phone the list is the whole story.
  const [showTimeline, setShowTimeline] = useState(false);
  const [showAddItem, setShowAddItem] = useState(false);
  const [addName, setAddName] = useState('');
  const [addQuantity, setAddQuantity] = useState('');
  const [addExpiry, setAddExpiry] = useState('');
  const [addStorage, setAddStorage] = useState<NewLarderItem['storage']>('fridge');
  const [consumeItem, setConsumeItem] = useState<LarderItem | null>(null);
  const [consumeQuantity, setConsumeQuantity] = useState('');
  const [discardRemaining, setDiscardRemaining] = useState(false);
  const [consumeWasteReason, setConsumeWasteReason] = useState('leftover_not_eaten');
  const [wasteItem, setWasteItem] = useState<LarderItem | null>(null);
  const [wasteQuantity, setWasteQuantity] = useState('');
  const [wasteReason, setWasteReason] = useState('spoiled_visible');
  const [isSaving, setIsSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const timelineVisible = showTimeline && !isMobile;

  const byExpiry = [...items].sort((a, b) => a.daysLeft - b.daysLeft);
  const eatSoon = byExpiry.filter((item) => item.daysLeft <= EAT_SOON_DAYS);

  const bands = BANDS.map((band, i) => ({
    ...band,
    items: byExpiry.filter(
      (item) => item.daysLeft <= band.max && item.daysLeft > (BANDS[i - 1]?.max ?? -Infinity)
    ),
  })).filter((band) => band.items.length > 0);

  // Walk the timeline left to right, nudging each bubble right of the one
  // before it. The scale still decides where an item wants to sit; this only
  // resolves the collisions it creates at the crowded end.
  let previousPct = -Infinity;
  const placed = byExpiry.map((item, index) => {
    const leftPct = Math.max(timelineLeftPct(item.daysLeft), previousPct + MIN_GAP_PCT);
    previousPct = leftPct;
    return {
      item,
      leftPct,
      labelBottom: index % 2 === 0 ? LABEL_BOTTOM_BELOW : LABEL_BOTTOM_ABOVE,
      size: bubbleSize(item.daysLeft),
    };
  });

  const resetForms = () => {
    setShowAddItem(false);
    setConsumeItem(null);
    setWasteItem(null);
    setFormError(null);
  };

  const handleAddItem = async (event: React.FormEvent) => {
    event.preventDefault();
    const quantity = Number(addQuantity);
    if (!addName.trim() || !Number.isFinite(quantity) || quantity <= 0) {
      setFormError('Enter a name and a quantity greater than zero.');
      return;
    }
    setIsSaving(true);
    setFormError(null);
    try {
      await onAddItem({ name: addName.trim(), quantityPurchased: quantity, expiryDate: addExpiry, storage: addStorage });
      setAddName('');
      setAddQuantity('');
      setAddExpiry('');
      resetForms();
    } catch (error) {
      console.error(error);
      setFormError('The item could not be saved. Check your connection and try again.');
    } finally {
      setIsSaving(false);
    }
  };

  const openWasteForm = (item: LarderItem) => {
    const remaining = (item.quantityPurchased ?? 0) - (item.quantityConsumed ?? 0) - (item.quantityWasted ?? 0);
    setWasteItem(item);
    setWasteQuantity(String(Math.min(remaining, 100)));
    setWasteReason('spoiled_visible');
    setConsumeItem(null);
    setShowAddItem(false);
    setFormError(null);
  };

  const openConsumeForm = (item: LarderItem) => {
    const remaining = (item.quantityPurchased ?? 0) -
      (item.quantityConsumed ?? 0) - (item.quantityWasted ?? 0);
    setConsumeItem(item);
    setConsumeQuantity(String(Math.min(remaining, 100)));
    setDiscardRemaining(item.sourceType === 'food' || item.sourceType === 'product');
    setConsumeWasteReason(item.sourceType === 'ingredient' ? 'other' : 'leftover_not_eaten');
    setWasteItem(null);
    setShowAddItem(false);
    setFormError(null);
  };

  const handleConsumeItem = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!consumeItem) return;
    const quantity = Number(consumeQuantity);
    const remaining = (consumeItem.quantityPurchased ?? 0) -
      (consumeItem.quantityConsumed ?? 0) - (consumeItem.quantityWasted ?? 0);
    if (!Number.isFinite(quantity) || quantity < 0 || quantity > remaining || (quantity === 0 && !discardRemaining)) {
      setFormError(`Enter a consumed quantity from 0 to ${remaining.toLocaleString('en-US')} g. Zero is only valid when discarding the remainder.`);
      return;
    }
    setIsSaving(true);
    setFormError(null);
    try {
      await onConsumeItem(consumeItem, quantity, discardRemaining, consumeWasteReason);
      resetForms();
    } catch (error) {
      console.error(error);
      setFormError('The consumption could not be saved. Check your connection and try again.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleWasteItem = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!wasteItem) return;
    const quantity = Number(wasteQuantity);
    const remaining = (wasteItem.quantityPurchased ?? 0) -
      (wasteItem.quantityConsumed ?? 0) - (wasteItem.quantityWasted ?? 0);
    if (!Number.isFinite(quantity) || quantity <= 0 || quantity > remaining) {
      setFormError(`Enter a quantity from 1 to ${remaining.toLocaleString('en-US')} g.`);
      return;
    }
    setIsSaving(true);
    setFormError(null);
    try {
      await onWasteItem(wasteItem, quantity, wasteReason);
      resetForms();
    } catch (error) {
      console.error(error);
      setFormError('The waste event could not be saved. Check your connection and try again.');
    } finally {
      setIsSaving(false);
    }
  };

  const totalWaste = wasteEvents.reduce((total, event) => total + event.quantity, 0);
  const totalCO2e = wasteEvents.reduce((total, event) => total + event.impactKgCO2e, 0);
  const totalVirtualWater = wasteEvents.reduce((total, event) => total + event.virtualWaterL, 0);
  const totalTreeEquivalents = wasteEvents.reduce((total, event) => total + event.treeEquivalents, 0);
  const latestWasteAt = wasteEvents.reduce((latest, event) => Math.max(latest, event.wastedAt), 0);
  const zeroWasteDays = latestWasteAt ? Math.max(0, Math.floor((Date.now() - latestWasteAt) / 86_400_000)) : null;

  const drivers = impactDrivers(wasteEvents, totalCO2e);
  const supportingImpact = [
    {
      key: 'water',
      figure: `${totalVirtualWater.toLocaleString('en-US', { maximumFractionDigits: 0 })} L`,
      caption: 'estimated virtual water',
      badge: (
        <span className="relative grid h-11 w-11 flex-none place-items-center overflow-hidden rounded-full band-fill">
          {/* Ambient only: the level is a constant, not a reading of the litres
              beside it. Nothing here is a second way to say the number. */}
          <svg
            className="pointer-events-none absolute inset-x-0 bottom-0 h-[58%] w-full"
            viewBox="0 0 80 24"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            {/* Painted with SVG attributes rather than `fill-bg/40`: Tailwind
                cannot put an opacity modifier on a bare var() colour, so that
                class emits no rule and the path falls back to black. */}
            <path className="water-wave" d={WAVE_PATH} fill="var(--color-bg)" opacity="0.4" />
            <g transform="translate(-13 3.5)">
              <path
                className="water-wave water-wave-slow"
                d={WAVE_PATH}
                fill="var(--color-bg)"
                opacity="0.25"
              />
            </g>
          </svg>
          <Droplets size={19} strokeWidth={2.5} className="relative" aria-hidden="true" />
        </span>
      ),
    },
    {
      key: 'trees',
      figure: totalTreeEquivalents.toLocaleString('en-US', {
        minimumFractionDigits: totalTreeEquivalents > 0 && totalTreeEquivalents < 0.01 ? 3 : 0,
        maximumFractionDigits: 3,
      }),
      caption: 'urban tree-year equivalents',
      badge: (
        <span className="grid h-11 w-11 flex-none place-items-center rounded-full band-fill">
          <Trees size={19} strokeWidth={2.5} aria-hidden="true" />
        </span>
      ),
    },
  ];

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-5 duration-500 md:gap-6">
      {/* Header */}
      <div className="flex flex-wrap items-end gap-x-3 gap-y-4 pt-2 md:gap-4">
        <header className="mr-auto">
          <h1 className="text-4xl text-ink md:text-[44px]">The larder</h1>
          <p className="mt-1.5 text-sm text-neutral-700 tabular-nums">
            {isLoading
              ? 'Loading your saved larder…'
              : `${items.length} items tracked · ${zeroWasteDays === null ? 'no waste logged yet' : `nothing wasted in ${zeroWasteDays} days`}`}
          </p>
        </header>
        <span className="tag tag-accent px-3.5 py-1.5 tabular-nums">{eatSoon.length} to eat soon</span>
        <button
          type="button"
          onClick={() => {
            setShowAddItem((current) => !current);
            setConsumeItem(null);
            setWasteItem(null);
            setFormError(null);
          }}
          className="btn btn-primary"
        >
          <Plus size={16} strokeWidth={2.5} />
          Add item
        </button>
      </div>

      {showAddItem && (
        <form onSubmit={handleAddItem} className="panel grid grid-cols-1 gap-4 p-5 sm:grid-cols-2 lg:grid-cols-5">
          <div className="lg:col-span-2">
            <label htmlFor="pantry-item-name" className="field-label">Item name</label>
            <input
              id="pantry-item-name"
              className="input"
              value={addName}
              onChange={(event) => setAddName(event.target.value)}
              placeholder="Baby spinach"
              autoFocus
              required
            />
          </div>
          <div>
            <label htmlFor="pantry-item-quantity" className="field-label">Quantity (g)</label>
            <input
              id="pantry-item-quantity"
              type="number"
              min="0.01"
              step="0.01"
              className="input tabular-nums"
              value={addQuantity}
              onChange={(event) => setAddQuantity(event.target.value)}
              required
            />
          </div>
          <div>
            <label htmlFor="pantry-item-expiry" className="field-label">Best before</label>
            <input
              id="pantry-item-expiry"
              type="date"
              className="input tabular-nums"
              value={addExpiry}
              onChange={(event) => setAddExpiry(event.target.value)}
            />
          </div>
          <div>
            <label htmlFor="pantry-item-storage" className="field-label">Storage</label>
            <select
              id="pantry-item-storage"
              className="input"
              value={addStorage}
              onChange={(event) => setAddStorage(event.target.value as NewLarderItem['storage'])}
            >
              <option value="fridge">Fridge</option>
              <option value="freezer">Freezer</option>
              <option value="pantry">Pantry</option>
              <option value="other">Other</option>
            </select>
          </div>
          {formError && <p role="alert" className="text-sm font-semibold text-accent-800 sm:col-span-2 lg:col-span-5">{formError}</p>}
          <div className="flex justify-end gap-2 sm:col-span-2 lg:col-span-5">
            <button type="button" onClick={resetForms} className="btn btn-secondary">Cancel</button>
            <button type="submit" disabled={isSaving} className="btn btn-primary">
              {isSaving && <Loader2 size={16} className="animate-spin" />}
              {isSaving ? 'Saving…' : 'Save item'}
            </button>
          </div>
        </form>
      )}

      {consumeItem && (
        <form onSubmit={handleConsumeItem} className="panel grid grid-cols-1 gap-4 p-5 sm:grid-cols-2">
          <div>
            <label htmlFor="consume-quantity" className="field-label">Consumed from {consumeItem.name} (g)</label>
            <input
              id="consume-quantity"
              type="number"
              min="0"
              step="0.01"
              className="input tabular-nums"
              value={consumeQuantity}
              onChange={(event) => setConsumeQuantity(event.target.value)}
              aria-describedby="consume-help"
              autoFocus
              required
            />
          </div>
          <div className="min-w-0 rounded-2xl bg-bg px-4 py-3">
            <label className="flex min-h-11 cursor-pointer items-start gap-3 text-sm font-semibold text-ink">
              <input
                type="checkbox"
                className="mt-1 h-4 w-4 flex-none accent-[var(--color-accent-700)]"
                checked={discardRemaining}
                onChange={(event) => setDiscardRemaining(event.target.checked)}
              />
              <span>Discard everything left after this amount</span>
            </label>
            <p id="consume-help" className="mt-1 break-words text-xs leading-relaxed text-neutral-700">
              {consumeItem.sourceType === 'ingredient'
                ? discardRemaining
                  ? 'Consumed nutrition will be added; the rest will move to waste.'
                  : 'Consumed nutrition will be added. The rest stays here until its estimated expiry.'
                : discardRemaining
                  ? 'The remaining portion will leave the provisional nutrition total and move to waste.'
                  : 'The remaining portion stays here. Its nutrition can be added when you consume it later.'}
            </p>
          </div>
          {discardRemaining && (
            <div className="sm:col-span-2">
              <label htmlFor="consume-waste-reason" className="field-label">Reason for discarding the remainder</label>
              <select
                id="consume-waste-reason"
                className="input"
                value={consumeWasteReason}
                onChange={(event) => setConsumeWasteReason(event.target.value)}
              >
                <option value="leftover_not_eaten">Leftover not eaten</option>
                <option value="spoiled_visible">Visible spoilage</option>
                <option value="expired_best_before">Past best-before date</option>
                <option value="expired_use_by">Past use-by date</option>
                <option value="forgot_item_existed">Forgot it was there</option>
                <option value="overbought">Bought too much</option>
                <option value="other">Other</option>
              </select>
            </div>
          )}
          {formError && <p role="alert" className="text-sm font-semibold text-accent-800 sm:col-span-2">{formError}</p>}
          <div className="flex flex-wrap justify-end gap-2 sm:col-span-2">
            <button type="button" onClick={resetForms} className="btn btn-secondary">Cancel</button>
            <button type="submit" disabled={isSaving} className="btn btn-primary">
              {isSaving && <Loader2 size={16} className="animate-spin" aria-hidden="true" />}
              {isSaving ? 'Saving…' : 'Save consumption'}
            </button>
          </div>
        </form>
      )}

      {wasteItem && (
        <form onSubmit={handleWasteItem} className="panel grid grid-cols-1 gap-4 p-5 sm:grid-cols-3">
          <div>
            <label htmlFor="waste-quantity" className="field-label">Waste from {wasteItem.name} (g)</label>
            <input
              id="waste-quantity"
              type="number"
              min="0.01"
              step="0.01"
              className="input tabular-nums"
              value={wasteQuantity}
              onChange={(event) => setWasteQuantity(event.target.value)}
              autoFocus
              required
            />
          </div>
          <div className="sm:col-span-2">
            <label htmlFor="waste-reason" className="field-label">Reason</label>
            <select id="waste-reason" className="input" value={wasteReason} onChange={(event) => setWasteReason(event.target.value)}>
              <option value="spoiled_visible">Visible spoilage</option>
              <option value="expired_best_before">Past best-before date</option>
              <option value="expired_use_by">Past use-by date</option>
              <option value="forgot_item_existed">Forgot it was there</option>
              <option value="overbought">Bought too much</option>
              <option value="leftover_not_eaten">Leftover not eaten</option>
              <option value="other">Other</option>
            </select>
          </div>
          {formError && <p role="alert" className="text-sm font-semibold text-accent-800 sm:col-span-3">{formError}</p>}
          <div className="flex justify-end gap-2 sm:col-span-3">
            <button type="button" onClick={resetForms} className="btn btn-secondary">Cancel</button>
            <button type="submit" disabled={isSaving} className="btn btn-primary">
              {isSaving && <Loader2 size={16} className="animate-spin" />}
              {isSaving ? 'Saving…' : 'Log waste'}
            </button>
          </div>
        </form>
      )}

      {/* The footprint of what was thrown out. This is the argument the larder
          screen exists to make, so it is the loudest surface on the page: an
          inverted sage band with no twin elsewhere in the app, above the list
          rather than under it. Both themes flip the accent-2 ramp, so
          bg/accent-2-900 stays a high-contrast pair in each. */}
      <section
        className="rounded-card bg-accent-2-900 px-6 py-6 text-bg shadow-md md:px-8 md:py-7"
        aria-label="Waste impact totals"
      >
        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <h2 className="kicker band-muted">What the waste cost</h2>
          <p className="text-xs band-muted">Estimated from published category factors</p>
        </div>

        <div className="mt-5 grid gap-7 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)] lg:gap-12">
          <div>
            <p className="font-display text-[46px] leading-none tabular-nums sm:text-[54px] md:text-[64px]">
              {totalCO2e.toLocaleString('en-US', { maximumFractionDigits: 2 })} kg
            </p>
            <p className="mt-2.5 text-[15px] font-semibold">estimated CO₂e from waste</p>
            <p className="mt-1 text-[13px] band-muted tabular-nums">
              <span className="font-semibold text-bg">
                {totalWaste.toLocaleString('en-US', { maximumFractionDigits: 1 })} g
              </span>{' '}
              thrown out
            </p>
          </div>

          <div className="flex flex-col justify-center">
            {drivers.length > 0 ? (
              <>
                <p className="text-[13px] font-semibold band-muted">Where it came from</p>
                {/* Rank, not hue: tinting one colour keeps the segments legible
                    in both themes, and the list below carries the same figures
                    as text. */}
                <div
                  className="impact-bar mt-2.5 flex h-3.5 overflow-hidden rounded-full band-fill"
                  aria-hidden="true"
                >
                  {drivers.map((driver, index) => (
                    <div
                      key={driver.name}
                      className="h-full bg-bg transition-[width] duration-700"
                      style={{ width: `${driver.sharePct}%`, opacity: DRIVER_OPACITY[index] }}
                    />
                  ))}
                </div>
                <ul className="mt-3 flex flex-col gap-1.5">
                  {drivers.map((driver, index) => (
                    <li
                      key={driver.name}
                      className="impact-row flex items-center gap-2.5 text-[13px]"
                      // Rows land behind the wipe that reveals their segment.
                      style={{ animationDelay: `${260 + index * 70}ms` }}
                    >
                      <span
                        className="h-2 w-2 flex-none rounded-full bg-bg"
                        style={{ opacity: DRIVER_OPACITY[index] }}
                        aria-hidden="true"
                      />
                      <span className="min-w-0 flex-1 truncate">{driver.name}</span>
                      <span className="flex-none band-muted tabular-nums">
                        {driver.quantity.toLocaleString('en-US', { maximumFractionDigits: 0 })} g ·{' '}
                        {Math.round(driver.sharePct)}%
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <p className="max-w-[42ch] text-[15px] leading-relaxed band-muted">
                {/* Saved events can carry no footprint — the mapper defaults a
                    missing estimate to zero — so the empty copy has to answer
                    for what was actually logged. */}
                {wasteEvents.length === 0
                  ? 'Nothing thrown out yet. When you log a discard, this shows what it cost and which food carried most of it.'
                  : 'No footprint estimate came back for what you logged, so there is nothing to break down yet.'}
              </p>
            )}
          </div>
        </div>

        <div className="mt-6 grid grid-cols-1 gap-4 border-t band-rule pt-5 sm:grid-cols-2">
          {supportingImpact.map(({ key, figure, caption, badge }) => (
            <div key={key} className="flex items-center gap-3.5">
              {badge}
              <span className="min-w-0">
                <span className="block font-display text-[22px] leading-tight tabular-nums">{figure}</span>
                <span className="block text-[13px] band-muted">{caption}</span>
              </span>
            </div>
          ))}
        </div>
      </section>

      <section className="grid grid-cols-1 items-start gap-5 lg:grid-cols-[2.1fr_1fr]">
        {/* The larder itself — primary content, so it takes the elevated card. */}
        <div className="card p-6 md:px-7" onMouseEnter={onHoverStart} onMouseLeave={onHoverEnd}>
          <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-2">
            <h2 className="text-[21px] text-ink">Sorted by what turns first</h2>
            {/* Offered only where the canvas fits. */}
            {!isMobile && (
              <div className="inline-flex overflow-hidden rounded-full border border-divider" role="group" aria-label="Larder layout">
                <button
                  type="button"
                  aria-pressed={!showTimeline}
                  onClick={() => setShowTimeline(false)}
                  className={`flex items-center gap-1.5 px-3.5 py-2 text-[12.5px] transition-colors ${
                    !showTimeline ? 'bg-accent-solid font-semibold text-bg' : 'text-neutral-700 hover:bg-neutral-200'
                  }`}
                >
                  <Rows3 size={14} strokeWidth={2.5} />
                  List
                </button>
                <div className="w-px bg-divider" />
                <button
                  type="button"
                  aria-pressed={showTimeline}
                  onClick={() => setShowTimeline(true)}
                  className={`flex items-center gap-1.5 px-3.5 py-2 text-[12.5px] transition-colors ${
                    showTimeline ? 'bg-accent-solid font-semibold text-bg' : 'text-neutral-700 hover:bg-neutral-200'
                  }`}
                >
                  <LineChart size={14} strokeWidth={2.5} />
                  Timeline
                </button>
              </div>
            )}
          </div>

          {timelineVisible ? (
            <>
              <p className="mt-1 text-[13px] text-neutral-700">Position is days until it turns.</p>
              <div className="relative mt-4 h-[196px]">
                <div
                  className="absolute inset-x-0 rounded-full opacity-55"
                  style={{
                    bottom: BUBBLE_BOTTOM + 4,
                    height: 44,
                    background:
                      'linear-gradient(90deg, var(--color-accent-400), var(--color-accent-300) 26%, var(--color-accent-2-200) 55%, var(--color-accent-2-300))',
                  }}
                />
                {placed.map(({ item, leftPct, size }) => (
                  <div
                    key={item.id}
                    className="absolute grid -translate-x-1/2 place-items-center rounded-full bg-neutral-100 text-ink shadow-sm"
                    style={{
                      left: `${leftPct}%`,
                      bottom: BUBBLE_BOTTOM,
                      width: size,
                      height: size,
                      fontSize: size >= 52 ? 17 : 15,
                      fontWeight: 600,
                      border: `3px solid ${urgencyBorder(item.daysLeft)}`,
                    }}
                    aria-hidden="true"
                  >
                    {item.monogram}
                  </div>
                ))}
                {/* Labels drawn after every bubble so they stay legible in tight clusters. */}
                {placed.map(({ item, leftPct, labelBottom }) => (
                  <div
                    key={`${item.id}-label`}
                    className={`absolute w-20 -translate-x-1/2 text-center text-xs leading-snug tabular-nums ${
                      item.daysLeft <= EAT_SOON_DAYS ? 'font-semibold text-accent-800' : 'text-neutral-700'
                    }`}
                    style={{ left: `${leftPct}%`, bottom: labelBottom }}
                  >
                    {item.label} · {daysLabel(item.daysLeft)}
                  </div>
                ))}
              </div>
              <div className="mt-1 flex justify-between text-xs text-neutral-700">
                <span className="font-semibold text-accent-700">eat now</span>
                <span>this week</span>
                <span>keeps well</span>
              </div>
              {/* The canvas is decorative once the same facts are in the list;
                  this keeps them reachable without switching views. */}
              <ul className="sr-only">
                {byExpiry.map((item) => (
                  <li key={item.id}>{`${item.name}, ${plainDaysLeft(item.daysLeft)}, turns ${turnsOn(item.daysLeft)}`}</li>
                ))}
              </ul>
            </>
          ) : (
            <div className="mt-3 flex flex-col gap-5">
              {!isLoading && bands.length === 0 && (
                <div className="rounded-3xl bg-bg px-5 py-10 text-center">
                  <p className="font-display text-xl text-ink">Your larder is empty.</p>
                  <p className="mt-1 text-sm text-neutral-700">Add an item to start tracking what should be used first.</p>
                </div>
              )}
              {bands.map((band) => (
                <section key={band.id} aria-labelledby={`larder-band-${band.id}`}>
                  <div className="flex items-baseline justify-between gap-3 border-b border-divider pb-1.5">
                    <h3 id={`larder-band-${band.id}`} className="kicker text-accent-700">
                      {band.title}
                    </h3>
                    <span className="text-xs text-neutral-700">{band.caption}</span>
                  </div>
                  <ul className="divide-y divide-divider">
                    {band.items.map((item) => (
                      <LarderRow key={item.id} item={item} onConsume={openConsumeForm} onWaste={openWasteForm} />
                    ))}
                  </ul>
                </section>
              ))}
            </div>
          )}
        </div>

        {/* Tonight's suggestion — the one action on this screen, so it keeps a
            filled treatment of its own. */}
        {eatSoon.length > 0 && <div className="flex flex-col gap-1.5 rounded-card bg-accent-2-200 px-6 py-5">
          <span className="kicker text-accent-2-800">
            Tonight, use {eatSoon.length === 1 ? 'this' : `all ${eatSoon.length}`}
          </span>
          <h2 className="font-display text-xl leading-tight text-accent-2-900">
            Spinach &amp; yogurt flatbreads, berries after
          </h2>
          <p className="text-[13px] text-accent-2-900 tabular-nums">620 kcal · 34 g protein · fits today&apos;s budget</p>
          <p className="mt-1 text-[13px] text-accent-2-900">
            Uses {eatSoon.map((item) => item.name.toLowerCase()).join(', ')}.
          </p>
          <div className="mt-2.5">
            <PlannedAction
              id="pantry-cook-this"
              label="Cook this"
              note="Full recipes are not in the app yet."
              className="border-accent-2-800/40 text-accent-2-900"
            />
          </div>
        </div>}
      </section>

      {wasteEvents.length > 0 && (
        <section className="panel px-6 py-5" aria-labelledby="waste-history-heading">
          <h2 id="waste-history-heading" className="text-[21px] text-ink">Recent waste</h2>
          <ul className="mt-2 divide-y divide-divider">
            {wasteEvents.slice(0, 5).map((event) => (
              <li key={event.id} className="flex min-w-0 flex-col gap-1 py-3 text-sm sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                <span className="min-w-0 break-words">
                  <span className="font-semibold text-ink">{event.foodName}</span>
                  <span className="ml-2 text-neutral-600">{event.reason.replace(/_/g, ' ')}</span>
                  {event.category && <span className="ml-2 text-xs text-neutral-600">· {event.category}</span>}
                </span>
                <span className="flex-none self-end text-neutral-700 tabular-nums sm:self-auto">
                  {event.quantity.toLocaleString('en-US')} g · {new Date(event.wastedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
};

export default PantryView;
