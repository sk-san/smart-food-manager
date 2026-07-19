import React from 'react';
import { Clock3, Leaf, Plus } from 'lucide-react';
import { SEED_LARDER, EAT_SOON_DAYS, ZERO_WASTE_STREAK_DAYS, LarderItem } from '../types/pantry';

interface PantryViewProps {
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

// Vertical scatter for the bubbles, cycled by index so neighbors stagger.
const BUBBLE_BOTTOMS = [64, 104, 52, 84, 110, 70, 96, 66];

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

const PantryView: React.FC<PantryViewProps> = ({ onHoverStart, onHoverEnd }) => {
  const items = SEED_LARDER;
  const eatSoon = items
    .filter((item) => item.daysLeft <= EAT_SOON_DAYS)
    .sort((a, b) => a.daysLeft - b.daysLeft);

  // Spread items that share a daysLeft value so their bubbles don't stack.
  const seenAtDay = new Map<number, number>();
  const placed = items.map((item, index) => {
    const dupes = seenAtDay.get(item.daysLeft) ?? 0;
    seenAtDay.set(item.daysLeft, dupes + 1);
    return {
      item,
      leftPct: timelineLeftPct(item.daysLeft) + dupes * 5.5,
      bottomPx: BUBBLE_BOTTOMS[index % BUBBLE_BOTTOMS.length],
      size: bubbleSize(item.daysLeft),
    };
  });

  const prevMonth = new Date(new Date().setDate(0)).toLocaleDateString('en-US', { month: 'long' });

  const impact = [
    {
      key: 'waste',
      figure: '−64%',
      caption: `food waste vs. ${prevMonth}`,
      visual: (
        <svg width="90" height="38" viewBox="0 0 90 38" aria-hidden="true">
          <polyline
            points="2,30 15,26 28,28 41,18 54,20 67,10 88,6"
            fill="none"
            stroke="var(--color-accent-2-600)"
            strokeWidth="3"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      ),
    },
    {
      key: 'streak',
      figure: `${ZERO_WASTE_STREAK_DAYS} days`,
      caption: 'zero-waste streak',
      visual: (
        <div className="grid h-11 w-11 flex-none place-items-center rounded-full bg-accent-2-200">
          <Clock3 size={21} strokeWidth={2.75} className="text-accent-2-800" />
        </div>
      ),
    },
    {
      key: 'co2',
      figure: '4.1 kg',
      caption: 'CO₂e avoided this month',
      visual: (
        <div className="grid h-11 w-11 flex-none place-items-center rounded-full bg-accent-200">
          <Leaf size={21} strokeWidth={2.75} className="text-accent-800" />
        </div>
      ),
    },
  ];

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-5 duration-500 md:gap-6">
      {/* Header */}
      <div className="flex flex-wrap items-end gap-3 pt-2 md:gap-4">
        <header className="mr-auto">
          <h1 className="text-4xl text-ink md:text-[44px]">The larder</h1>
          <p className="mt-1.5 text-sm text-neutral-600 tabular-nums">
            {items.length} items tracked · nothing wasted in {ZERO_WASTE_STREAK_DAYS} days
          </p>
        </header>
        <span className="tag tag-accent px-3.5 py-1.5 text-xs tabular-nums">{eatSoon.length} to eat soon</span>
        <span className="tag tag-accent-2 hidden px-3.5 py-1.5 text-xs sm:inline-flex">€23 saved this month</span>
        <button className="btn btn-primary shadow-sm">
          <Plus size={15} strokeWidth={2.75} />
          Add item
        </button>
      </div>

      <section className="grid grid-cols-1 items-stretch gap-5 lg:grid-cols-[2.1fr_1fr]">
        {/* Ripeness timeline */}
        <div
          className="rounded-card bg-surface p-6 shadow-sm md:px-7"
          onMouseEnter={onHoverStart}
          onMouseLeave={onHoverEnd}
        >
          <div className="flex items-baseline justify-between gap-3">
            <span className="kicker text-accent">Ripeness timeline</span>
            <span className="text-xs text-neutral-600">position = days until it turns</span>
          </div>

          <div className="-mx-2 overflow-x-auto px-2">
          <div className="relative mt-4 h-[210px] min-w-[560px]">
            <div
              className="absolute inset-x-0 bottom-[34px] h-[110px] rounded-full opacity-55"
              style={{
                background:
                  'linear-gradient(90deg, var(--color-accent-400), var(--color-accent-300) 26%, var(--color-accent-2-200) 55%, var(--color-accent-2-300))',
              }}
            />
            {placed.map(({ item, leftPct, bottomPx, size }) => (
              <div
                key={item.id}
                className="absolute grid -translate-x-1/2 place-items-center rounded-full bg-neutral-100 font-display text-ink shadow-sm"
                style={{
                  left: `${leftPct}%`,
                  bottom: bottomPx,
                  width: size,
                  height: size,
                  fontSize: size >= 52 ? 19 : 16,
                  border: `3px solid ${urgencyBorder(item.daysLeft)}`,
                }}
                title={`${item.name} · ${daysLabel(item.daysLeft)}`}
              >
                {item.monogram}
              </div>
            ))}
            {/* Labels drawn after every bubble so they stay legible in tight clusters. */}
            {placed.map(({ item, leftPct, bottomPx }) => (
              <div
                key={`${item.id}-label`}
                className={`absolute w-16 -translate-x-1/2 text-center text-[11px] tabular-nums ${
                  item.daysLeft <= EAT_SOON_DAYS ? 'font-semibold text-accent-800' : 'text-neutral-700'
                }`}
                style={{ left: `${leftPct}%`, bottom: bottomPx - 34 }}
              >
                {item.label} · {daysLabel(item.daysLeft)}
              </div>
            ))}
          </div>
          </div>

          <div className="mt-1 flex justify-between text-[11.5px] text-neutral-600">
            <span className="font-semibold text-accent-700">eat now</span>
            <span>this week</span>
            <span>keeps well</span>
          </div>
        </div>

        {/* Eat-soon queue feeding a recipe */}
        <div className="flex flex-col gap-3.5">
          <div
            className="rounded-card bg-surface px-6 py-4 shadow-sm"
            onMouseEnter={onHoverStart}
            onMouseLeave={onHoverEnd}
          >
            <span className="kicker text-accent">Eat soon</span>
            <div className="mt-1 divide-y divide-divider">
              {eatSoon.map((item: LarderItem) => (
                <div key={item.id} className="flex items-center gap-3 py-2">
                  <span className="tag tag-accent flex-none tabular-nums">
                    {item.daysLeft} {item.daysLeft === 1 ? 'day' : 'days'}
                  </span>
                  <span className="flex-1 truncate text-sm font-semibold text-ink">{item.name}</span>
                  <button className="btn btn-ghost px-2 text-[12.5px]">Recipes</button>
                </div>
              ))}
            </div>
          </div>

          <div className="flex flex-1 flex-col gap-1.5 rounded-card bg-accent-2-200 px-6 py-5">
            <span className="kicker text-accent-2-800">Tonight, use all three</span>
            <div className="font-display text-xl leading-tight text-accent-2-900">
              Spinach &amp; yogurt flatbreads, berries after
            </div>
            <div className="text-[12.5px] text-accent-2-800 tabular-nums">
              620 kcal · 34 g protein · fits today's budget
            </div>
            <button className="btn btn-primary mt-1.5 self-start text-[13px] shadow-sm">Cook this</button>
          </div>
        </div>
      </section>

      {/* Impact row */}
      <section className="grid grid-cols-1 gap-5 md:grid-cols-3">
        {impact.map(({ key, figure, caption, visual }) => (
          <div key={key} className="flex items-center gap-4 rounded-card bg-surface px-6 py-4 shadow-sm">
            {visual}
            <div>
              <div className="font-display text-[22px] text-ink tabular-nums">{figure}</div>
              <div className="text-xs text-neutral-600">{caption}</div>
            </div>
          </div>
        ))}
      </section>
    </div>
  );
};

export default PantryView;
