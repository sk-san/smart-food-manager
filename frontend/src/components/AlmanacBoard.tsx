import React from 'react';
import { Plus } from 'lucide-react';
import { FoodEntry, DailyGoal, NutritionData, DEMO_ACCOUNT_STATS } from '../types/nutrition';
import photo from '../assets/photo.jpg';

interface AlmanacBoardProps {
  todayTotals: NutritionData;
  goals: DailyGoal;
  entries: FoodEntry[];
  /** Kcal per day for the running week, today last — for the sparkline. */
  weekValues: number[];
  onLogFood: () => void;
  onOpenStats: () => void;
  onHoverStart: () => void;
  onHoverEnd: () => void;
}

// Sparkline geometry: week values plotted on a 150x44 canvas.
const SPARK_WIDTH = 150;
const SPARK_HEIGHT = 44;

/** Meal name for the log button, by time of day. */
const nextMealName = (hours: number) => (hours < 11 ? 'breakfast' : hours < 16 ? 'lunch' : 'supper');

const AlmanacBoard: React.FC<AlmanacBoardProps> = ({
  todayTotals,
  goals,
  entries,
  weekValues,
  onLogFood,
  onOpenStats,
  onHoverStart,
  onHoverEnd,
}) => {
  const calories = Math.round(todayTotals.calories);
  const remaining = goals.calories - calories;
  const meal = nextMealName(new Date().getHours());

  // The day, read as a sentence (design 1d: "hierarchy does the work").
  const lastEntry = entries.length > 0 ? entries[entries.length - 1] : null;
  const lastAt = lastEntry
    ? new Date(lastEntry.timestamp).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    : null;
  const tone =
    calories < goals.calories * 0.45 ? 'A light day so far' : calories <= goals.calories ? 'A steady day so far' : 'A generous day';
  const proteinClause =
    todayTotals.protein < goals.protein / 2
      ? 'most of the protein still to spend'
      : 'the protein already well settled';

  const macros = [
    { label: 'Protein', value: todayTotals.protein, goal: goals.protein, barColor: 'var(--color-accent-500)' },
    { label: 'Carbs', value: todayTotals.carbs, goal: goals.carbs, barColor: 'var(--color-accent-2-500)' },
    { label: 'Fat', value: todayTotals.fat, goal: goals.fat, barColor: 'var(--color-accent-700)' },
  ];

  // Sparkline points, normalized to the week's own range.
  const min = Math.min(...weekValues);
  const max = Math.max(...weekValues);
  const points = weekValues.map((kcal, i) => {
    const x = 4 + (i * (SPARK_WIDTH - 8)) / (weekValues.length - 1);
    const t = max > min ? (kcal - min) / (max - min) : 0.5;
    const y = SPARK_HEIGHT - 10 - t * (SPARK_HEIGHT - 18);
    return { x, y };
  });
  const lastPoint = points[points.length - 1];

  return (
    <div className="max-w-[540px]" onMouseEnter={onHoverStart} onMouseLeave={onHoverEnd}>
      <div className="flex items-baseline gap-2.5 pt-1">
        <span className="font-display text-[84px] leading-[0.95] tracking-[-0.02em] text-ink tabular-nums md:text-[104px]">
          {calories.toLocaleString('en-US')}
        </span>
        <div className="text-sm leading-snug text-neutral-700 tabular-nums">
          kcal so far,
          <br />
          of {goals.calories.toLocaleString('en-US')}
        </div>
      </div>

      <p className="mt-4 max-w-[36ch] text-[14.5px] leading-relaxed text-neutral-800">
        {entries.length === 0 ? (
          <>
            A blank page so far — nothing on the table yet. {meal[0].toUpperCase() + meal.slice(1)} has the whole{' '}
            <strong className="tabular-nums">{goals.calories.toLocaleString('en-US')} kcal</strong> and every gram of
            protein still to spend.
          </>
        ) : remaining >= 0 ? (
          <>
            {tone} — {entries.length} {entries.length === 1 ? 'meal' : 'meals'} logged, the last at {lastAt}. Supper has{' '}
            <strong className="tabular-nums">{remaining.toLocaleString('en-US')} kcal</strong> and {proteinClause}.
          </>
        ) : (
          <>
            {tone} — {entries.length} {entries.length === 1 ? 'meal' : 'meals'} logged, the last at {lastAt}. The page
            is over its {goals.calories.toLocaleString('en-US')} by{' '}
            <strong className="tabular-nums">{Math.abs(remaining).toLocaleString('en-US')} kcal</strong>; a lighter
            supper squares it.
          </>
        )}
      </p>

      {/* Macro rules */}
      <div className="mt-6 flex flex-col">
        {macros.map(({ label, value, goal, barColor }, i) => {
          const pct = goal > 0 ? Math.min(100, (value / goal) * 100) : 0;
          return (
            <div
              key={label}
              className={`flex items-center gap-3 border-t-2 border-dotted border-neutral-400 py-3 ${
                i === macros.length - 1 ? 'border-b-2' : ''
              }`}
            >
              <span className="w-[70px] text-[13px] font-semibold text-ink">{label}</span>
              <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-neutral-200">
                <div
                  className="h-full rounded-full"
                  style={{ width: `${pct}%`, background: barColor, transition: 'width 1s ease-in-out' }}
                />
              </div>
              <span className="w-[86px] text-right font-display text-[15px] text-ink tabular-nums">
                {Math.round(value)}{' '}
                <span className="font-sans text-[11px] text-neutral-600">/ {goal.toLocaleString('en-US')} g</span>
              </span>
            </div>
          );
        })}
      </div>

      {/* This week, beside a washed photo from the kitchen garden (design 1d) */}
      <div className="mt-6 flex items-center gap-[18px]">
        <img
          src={photo}
          alt="From the kitchen"
          className="washed h-[170px] w-[150px] flex-none object-cover"
          style={{ borderRadius: '58% 42% 53% 47% / 50% 58% 42% 50%' }}
        />
        <div>
          <div className="kicker font-semibold text-accent-2-700">This week</div>
          <svg
            width={SPARK_WIDTH}
            height={SPARK_HEIGHT}
            viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`}
            className="mb-1 mt-2"
            role="img"
            aria-label="Calories per day this week"
          >
            <polyline
              points={points.map(({ x, y }) => `${x},${y}`).join(' ')}
              fill="none"
              stroke="var(--color-accent-600)"
              strokeWidth="3"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <circle cx={lastPoint.x} cy={lastPoint.y} r="4.5" fill="var(--color-accent-600)" />
          </svg>
          <div className="text-[12.5px] leading-normal text-neutral-700">
            Averaging <strong className="tabular-nums">{DEMO_ACCOUNT_STATS.avgCalories.toLocaleString('en-US')} kcal</strong>.{' '}
            {DEMO_ACCOUNT_STATS.currentStreak} straight days inside the goal — the best run this month.
          </div>
        </div>
      </div>

      <div className="mt-7 flex gap-2.5">
        <button onClick={onLogFood} className="btn btn-primary flex-1 py-3 text-[15px] shadow-sm">
          <Plus size={16} strokeWidth={2.75} />
          Log {meal}
        </button>
        <button onClick={onOpenStats} className="btn btn-secondary flex-none px-5">
          Stats
        </button>
      </div>
    </div>
  );
};

export default AlmanacBoard;
