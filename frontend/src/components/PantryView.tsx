import React, { useState } from 'react';
import { Clock3, Leaf, LineChart, Rows3 } from 'lucide-react';
import PlannedAction from './PlannedAction';
import { useIsMobile } from '../hooks/useMediaQuery';
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

const LarderRow: React.FC<{ item: LarderItem }> = ({ item }) => {
  const urgent = item.daysLeft <= EAT_SOON_DAYS;
  return (
    <li className="flex items-center gap-3.5 py-3">
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
      </span>
      <span className={`tag flex-none tabular-nums ${urgent ? 'tag-accent font-semibold' : 'tag-neutral'}`}>
        {daysLabel(item.daysLeft)}
      </span>
    </li>
  );
};

const PantryView: React.FC<PantryViewProps> = ({ onHoverStart, onHoverEnd }) => {
  const items = SEED_LARDER;
  const isMobile = useIsMobile();
  // The timeline is a wide canvas by construction, so it is offered only where
  // there is width for it. On a phone the list is the whole story.
  const [showTimeline, setShowTimeline] = useState(false);
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

  const prevMonth = new Date(new Date().setDate(0)).toLocaleDateString('en-US', { month: 'long' });

  const impact = [
    {
      key: 'waste',
      figure: '−64%',
      caption: `food waste vs. ${prevMonth}`,
      visual: (
        <svg width="90" height="38" viewBox="0 0 90 38" aria-hidden="true" className="flex-none">
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
      <div className="flex flex-wrap items-end gap-x-3 gap-y-4 pt-2 md:gap-4">
        <header className="mr-auto">
          <h1 className="text-4xl text-ink md:text-[44px]">The larder</h1>
          <p className="mt-1.5 text-sm text-neutral-700 tabular-nums">
            {items.length} items tracked · nothing wasted in {ZERO_WASTE_STREAK_DAYS} days
          </p>
        </header>
        <span className="tag tag-accent px-3.5 py-1.5 tabular-nums">{eatSoon.length} to eat soon</span>
        <span className="tag tag-accent-2 hidden px-3.5 py-1.5 sm:inline-flex">€23 saved this month</span>
        <PlannedAction
          id="pantry-add-item"
          label="Add item"
          note="Larder contents are still seed data — editing arrives with pantry storage."
        />
      </div>

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
                      <LarderRow key={item.id} item={item} />
                    ))}
                  </ul>
                </section>
              ))}
            </div>
          )}
        </div>

        {/* Tonight's suggestion — the one action on this screen, so it keeps a
            filled treatment of its own. */}
        <div className="flex flex-col gap-1.5 rounded-card bg-accent-2-200 px-6 py-5">
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
        </div>
      </section>

      {/* Impact row — supporting figures, so open panels rather than cards. */}
      <section className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        {impact.map(({ key, figure, caption, visual }) => (
          <div key={key} className="panel flex items-center gap-4 px-5 py-4">
            {visual}
            <div>
              <div className="font-display text-[22px] text-ink tabular-nums">{figure}</div>
              <div className="text-[13px] text-neutral-700">{caption}</div>
            </div>
          </div>
        ))}
      </section>
    </div>
  );
};

export default PantryView;
