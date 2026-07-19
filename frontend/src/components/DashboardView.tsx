import React, { useEffect, useState } from 'react';
import { BookOpen, ChevronRight, CircleDashed, Rows3, TrendingUp } from 'lucide-react';
import NutritionCard from './NutritionCard';
import DayPlateBoard from './DayPlateBoard';
import AlmanacBoard from './AlmanacBoard';
import { FoodEntry, DailyGoal, NutritionData, DEMO_ACCOUNT_STATS } from '../types/nutrition';
import { logNavigation } from '../telemetry/events';

interface DashboardViewProps {
  todayTotals: NutritionData;
  goals: DailyGoal;
  entries: FoodEntry[];
  onLogFood: () => void;
  onOpenStats: () => void;
  onHoverStart: () => void;
  onHoverEnd: () => void;
}

// The dashboard renders one of three boards from the Dashboard Ideas doc:
// the Kitchen Ledger (1a), the Day Plate (1b) and the Almanac (1d). All read
// the same entries and goals; the choice is presentation, kept per browser.
// The Almanac is a phone-format page, so it is only offered on mobile widths.
const BOARDS = [
  { id: 'ledger', label: 'Ledger', icon: Rows3 },
  { id: 'plate', label: 'Plate', icon: CircleDashed },
  { id: 'almanac', label: 'Almanac', icon: BookOpen },
] as const;
type BoardId = (typeof BOARDS)[number]['id'];
const BOARD_STORAGE_KEY = 'nutri-dashboard-board';

// Below Tailwind's md breakpoint — the same cut the app's chrome uses to
// swap between the top nav and the bottom tab bar.
const MOBILE_QUERY = '(max-width: 767px)';

// Chart geometry for the "This week" card (design 1a): bars on a 240x130
// canvas, with the calorie goal drawn as a dashed line 90px above the base.
const CHART_BASE_Y = 120;
const CHART_GOAL_HEIGHT = 90;
const BAR_WIDTH = 30;
const BAR_XS = [14, 60, 106, 152, 198];

// Mock intake for the four days before today — real aggregation is a
// follow-up once entries persist through the backend.
const PREVIOUS_DAYS_KCAL = [1850, 2140, 1980, 2230];

const DashboardView: React.FC<DashboardViewProps> = ({
  todayTotals,
  goals,
  entries,
  onLogFood,
  onOpenStats,
  onHoverStart,
  onHoverEnd
}) => {
  const [selectedBoard, setSelectedBoard] = useState<BoardId>(() => {
    const stored = localStorage.getItem(BOARD_STORAGE_KEY);
    return BOARDS.some((b) => b.id === stored) ? (stored as BoardId) : 'ledger';
  });
  const [isMobile, setIsMobile] = useState(() => window.matchMedia(MOBILE_QUERY).matches);

  useEffect(() => {
    const mql = window.matchMedia(MOBILE_QUERY);
    const onChange = (e: MediaQueryListEvent) => setIsMobile(e.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, []);

  // On desktop widths an Almanac selection falls back to the ledger; the
  // stored preference survives, so a phone visit picks it back up.
  const visibleBoards = isMobile ? BOARDS : BOARDS.filter((b) => b.id !== 'almanac');
  const board: BoardId = !isMobile && selectedBoard === 'almanac' ? 'ledger' : selectedBoard;

  const handleBoardChange = (next: BoardId) => {
    if (next === board) return;
    logNavigation(`dashboard:${board}`, `dashboard:${next}`);
    setSelectedBoard(next);
    localStorage.setItem(BOARD_STORAGE_KEY, next);
  };

  const now = new Date();
  const weekday = now.toLocaleDateString('en-US', { weekday: 'long' });
  const monthDay = now.toLocaleDateString('en-US', { month: 'long', day: 'numeric' });

  const calories = Math.round(todayTotals.calories);
  const goalCalories = goals.calories;
  const remaining = goalCalories - calories;
  const onTrack = calories <= goalCalories;
  const progressPct = goalCalories > 0 ? Math.min(100, (calories / goalCalories) * 100) : 0;

  const weekValues = [...PREVIOUS_DAYS_KCAL, todayTotals.calories];
  const dayLabels = weekValues.map((_, i) => {
    const d = new Date();
    d.setDate(d.getDate() - (weekValues.length - 1 - i));
    return d.toLocaleDateString('en-US', { weekday: 'short' }).slice(0, 2);
  });
  const barHeight = (kcal: number) =>
    Math.max(4, Math.min(112, (kcal / goalCalories) * CHART_GOAL_HEIGHT));

  const minerals = [
    { label: 'Sodium', value: todayTotals.sodium, goal: goals.sodium, barClass: 'bg-accent-500' },
    { label: 'Calcium', value: todayTotals.calcium, goal: goals.calcium, barClass: 'bg-neutral-500' },
    { label: 'Iron', value: todayTotals.iron, goal: goals.iron, barClass: 'bg-accent-2-600' },
  ];

  const header =
    board === 'ledger' ? (
      <header>
        <h1 className="text-4xl text-ink md:text-[44px]">{weekday}, at the table</h1>
        <p className="mt-1.5 text-sm text-neutral-600 tabular-nums">
          {monthDay} · {calories.toLocaleString('en-US')} of {goalCalories.toLocaleString('en-US')} kcal logged
          {onTrack
            ? remaining > 0
              ? ` — ${remaining.toLocaleString('en-US')} kcal to go.`
              : ' — goal reached.'
            : ` — over by ${Math.abs(remaining).toLocaleString('en-US')} kcal.`}
        </p>
      </header>
    ) : board === 'plate' ? (
      <header>
        <h1 className="text-4xl text-ink md:text-[44px]">Your plate</h1>
        <p className="mt-1.5 text-sm text-neutral-600">
          {weekday}, {monthDay}
        </p>
      </header>
    ) : (
      <header className="kicker self-center font-semibold tracking-[0.14em] text-accent-700 tabular-nums">
        {weekday} · {monthDay} · entry no. {DEMO_ACCOUNT_STATS.daysLogged}
      </header>
    );

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-5 duration-500 md:gap-6">
      {/* Header + board switcher */}
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3 pt-2">
        {header}
        <div className="inline-flex overflow-hidden rounded-full border border-divider" role="tablist" aria-label="Dashboard view">
          {visibleBoards.map(({ id, label, icon: Icon }, i) => (
            <React.Fragment key={id}>
              {i > 0 && <div className="w-px bg-divider" />}
              <button
                role="tab"
                aria-selected={board === id}
                onClick={() => handleBoardChange(id)}
                className={`flex items-center gap-1.5 px-3.5 py-1.5 text-[12.5px] transition-colors ${
                  board === id ? 'bg-accent font-semibold text-bg' : 'text-neutral-700 hover:bg-neutral-200'
                }`}
              >
                <Icon size={14} strokeWidth={2.5} />
                <span>{label}</span>
              </button>
            </React.Fragment>
          ))}
        </div>
      </div>

      {board === 'plate' ? (
        <DayPlateBoard
          todayTotals={todayTotals}
          goals={goals}
          entries={entries}
          onHoverStart={onHoverStart}
          onHoverEnd={onHoverEnd}
        />
      ) : board === 'almanac' ? (
        <AlmanacBoard
          todayTotals={todayTotals}
          goals={goals}
          entries={entries}
          weekValues={weekValues.map((kcal) => Math.round(kcal))}
          onLogFood={onLogFood}
          onOpenStats={onOpenStats}
          onHoverStart={onHoverStart}
          onHoverEnd={onHoverEnd}
        />
      ) : (
        <>
          {/* Calories hero + week chart */}
          <section className="grid grid-cols-1 gap-5 lg:grid-cols-[1.6fr_1fr]">
            <div
              className="flex flex-col justify-between rounded-card bg-surface p-7 shadow-sm"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
            >
              <div className="flex items-start justify-between">
                <span className="kicker text-accent">Calories today</span>
                {onTrack ? (
                  <span className="tag tag-accent-2">on track</span>
                ) : (
                  <span className="tag tag-accent">over goal</span>
                )}
              </div>

              <div className="mb-1.5 mt-4 flex items-baseline gap-2.5">
                <span className="font-display text-6xl leading-none text-ink tabular-nums md:text-7xl">
                  {calories.toLocaleString('en-US')}
                </span>
                <span className="text-[17px] text-neutral-600 tabular-nums">
                  of {goalCalories.toLocaleString('en-US')} kcal
                </span>
              </div>

              <div className="mt-4">
                <div className="h-4 overflow-hidden rounded-full bg-neutral-200">
                  <div
                    className="h-full rounded-full transition-all duration-1000 ease-out"
                    style={{
                      width: `${progressPct}%`,
                      background: 'linear-gradient(90deg, var(--color-accent-2-400), var(--color-accent-500))',
                    }}
                  />
                </div>
                <div className="mt-2 flex justify-between text-xs text-neutral-600 tabular-nums">
                  <span>
                    {entries.length === 0
                      ? 'Nothing logged yet'
                      : `${entries.length} ${entries.length === 1 ? 'item' : 'items'} logged`}
                  </span>
                  <span>
                    {remaining >= 0
                      ? `${remaining.toLocaleString('en-US')} kcal left`
                      : `${Math.abs(remaining).toLocaleString('en-US')} kcal over`}
                  </span>
                </div>
              </div>
            </div>

            <div
              className="rounded-card bg-surface p-6 shadow-sm md:px-7"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
            >
              <div className="flex items-center justify-between">
                <span className="kicker text-accent-2-700">This week</span>
                <TrendingUp size={16} strokeWidth={2.75} className="text-accent-2-700" />
              </div>
              <svg viewBox="0 0 240 130" className="mt-1.5 h-auto w-full" role="img" aria-label="Calories per day this week">
                <line
                  x1="8"
                  y1={CHART_BASE_Y - CHART_GOAL_HEIGHT}
                  x2="232"
                  y2={CHART_BASE_Y - CHART_GOAL_HEIGHT}
                  strokeWidth="1.5"
                  strokeDasharray="3 5"
                  className="stroke-neutral-400"
                />
                {weekValues.map((kcal, i) => {
                  const h = barHeight(kcal);
                  const isToday = i === weekValues.length - 1;
                  const fill = isToday
                    ? 'var(--color-accent-500)'
                    : kcal >= goalCalories
                      ? 'var(--color-accent-2-400)'
                      : 'var(--color-accent-2-300)';
                  return (
                    <rect
                      key={i}
                      x={BAR_XS[i]}
                      y={CHART_BASE_Y - h}
                      width={BAR_WIDTH}
                      height={h}
                      rx="10"
                      fill={fill}
                    />
                  );
                })}
              </svg>
              <div className="flex justify-between px-2.5 text-[11px] text-neutral-600">
                {dayLabels.map((label, i) => (
                  <span key={i}>{label}</span>
                ))}
              </div>
              <p className="mt-2 text-xs text-neutral-600 tabular-nums">
                Dashed line is your {goalCalories.toLocaleString('en-US')} goal.
              </p>
            </div>
          </section>

          {/* Macro rings */}
          <section className="grid grid-cols-1 gap-5 md:grid-cols-3">
            <NutritionCard
              label="Protein"
              value={todayTotals.protein}
              goal={goals.protein}
              unit="g"
              ringColor="var(--color-accent-500)"
              kickerClass="text-accent"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
            />
            <NutritionCard
              label="Carbs"
              value={todayTotals.carbs}
              goal={goals.carbs}
              unit="g"
              ringColor="var(--color-accent-2-500)"
              kickerClass="text-accent-2-700"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
            />
            <NutritionCard
              label="Fat"
              value={todayTotals.fat}
              goal={goals.fat}
              unit="g"
              ringColor="var(--color-accent-700)"
              kickerClass="text-accent-700"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
            />
          </section>

          {/* Recent logs + minerals */}
          <section className="grid grid-cols-1 items-start gap-5 lg:grid-cols-[1.6fr_1fr]">
            <div
              className="rounded-card bg-surface p-6 shadow-sm md:px-7"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
            >
              <div className="mb-1.5 flex items-center justify-between">
                <h3 className="text-[21px] text-ink">Recent logs</h3>
                <button className="btn btn-ghost text-[13px]">
                  View all <ChevronRight size={14} strokeWidth={2.75} />
                </button>
              </div>

              {entries.length === 0 ? (
                <p className="py-6 text-center text-[13px] text-neutral-600">
                  Nothing on the table yet — tap <span className="font-semibold text-accent-700">Log food</span> to
                  start today&apos;s ledger.
                </p>
              ) : (
                <div className="divide-y divide-divider">
                  {entries
                    .slice()
                    .reverse()
                    .map((entry, index) => (
                      <div key={entry.id} className="flex items-center gap-4 py-3">
                        <div
                          className={`grid h-11 w-11 shrink-0 place-items-center rounded-full font-display text-lg ${
                            index % 2 === 0
                              ? 'bg-accent-2-200 text-accent-2-800'
                              : 'bg-accent-200 text-accent-800'
                          }`}
                        >
                          {entry.name.charAt(0).toUpperCase()}
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-[15px] font-semibold text-ink">{entry.name}</div>
                          <div className="text-xs text-neutral-600">
                            {new Date(entry.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                          </div>
                        </div>
                        <div className="hidden gap-1.5 sm:flex">
                          <span className="tag tag-accent tabular-nums">P {Math.round(entry.protein)}</span>
                          <span className="tag tag-accent-2 tabular-nums">C {Math.round(entry.carbs)}</span>
                          <span className="tag tag-neutral tabular-nums">F {Math.round(entry.fat)}</span>
                        </div>
                        <div className="w-[88px] text-right font-display text-[17px] text-ink tabular-nums">
                          {entry.calories.toLocaleString('en-US')} kcal
                        </div>
                      </div>
                    ))}
                </div>
              )}
            </div>

            <div className="rounded-card bg-surface p-6 shadow-sm md:px-7">
              <span className="kicker font-semibold text-accent-2-700">Minerals</span>
              <div className="mt-3 flex flex-col gap-4">
                {minerals.map(({ label, value, goal, barClass }) => {
                  const pct = goal > 0 ? Math.min(100, (value / goal) * 100) : 0;
                  return (
                    <div key={label}>
                      <div className="mb-1.5 flex items-baseline justify-between">
                        <span className="text-[13px] font-semibold text-ink">{label}</span>
                        <span className="text-xs text-neutral-600 tabular-nums">
                          {Math.round(value).toLocaleString('en-US')} / {goal.toLocaleString('en-US')} mg
                        </span>
                      </div>
                      <div className="h-2.5 overflow-hidden rounded-full bg-neutral-200">
                        <div
                          className={`h-full rounded-full ${barClass}`}
                          style={{ width: `${pct}%`, transition: 'width 1s ease-in-out' }}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
              <p className="mt-4 text-xs text-neutral-600">
                Targets come from your account goals — sodium reads as a limit, calcium and iron as minimums.
              </p>
            </div>
          </section>
        </>
      )}
    </div>
  );
};

export default DashboardView;
