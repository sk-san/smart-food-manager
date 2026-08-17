import React from "react";
import { ArrowRight, Camera, Clock3, Sparkles } from "lucide-react";
import companionArt from "../assets/nutri-companion.webp";
import type { DailyGoal, FoodEntry, NutritionData } from "../types/nutrition";

interface MobileDashboardViewProps {
  todayTotals: NutritionData;
  goals: DailyGoal;
  entries: FoodEntry[];
  firstName: string;
  onLogFood: () => void;
  onOpenStats: () => void;
}

interface ProgressBarProps {
  label: string;
  value: number;
  goal: number;
  unit: string;
  tone: "terracotta" | "sage" | "ink";
  kind?: "target" | "limit";
}

const TONE_CLASSES: Record<ProgressBarProps["tone"], string> = {
  terracotta: "bg-accent-600",
  sage: "bg-accent-2-600",
  ink: "bg-neutral-700",
};

const clampPercent = (value: number, goal: number) =>
  goal > 0 ? Math.min(100, Math.max(0, (value / goal) * 100)) : 0;

const ProgressBar: React.FC<ProgressBarProps> = ({
  label,
  value,
  goal,
  unit,
  tone,
  kind = "target",
}) => {
  const percentage = clampPercent(value, goal);
  const roundedValue = Math.round(value);
  const status =
    kind === "limit"
      ? `${Math.max(0, Math.round(goal - value)).toLocaleString("en-US")} ${unit} left`
      : `${Math.round(percentage)}% of target`;

  return (
    <div>
      <div className="mb-2 flex items-end justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-ink">{label}</p>
          <p className="text-xs text-neutral-600">{status}</p>
        </div>
        <p className="shrink-0 text-sm font-semibold text-ink tabular-nums">
          {roundedValue.toLocaleString("en-US")}
          <span className="ml-1 text-xs font-medium text-neutral-600">/ {goal.toLocaleString("en-US")} {unit}</span>
        </p>
      </div>
      <div
        className="h-2.5 overflow-hidden rounded-full bg-neutral-200"
        role="progressbar"
        aria-label={`${label}: ${roundedValue} of ${goal} ${unit}`}
        aria-valuemin={0}
        aria-valuemax={goal}
        aria-valuenow={Math.min(goal, roundedValue)}
      >
        <div
          className={`h-full rounded-full transition-[width] duration-700 ${TONE_CLASSES[tone]}`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  );
};

const MobileDashboardView: React.FC<MobileDashboardViewProps> = ({
  todayTotals,
  goals,
  entries,
  firstName,
  onLogFood,
  onOpenStats,
}) => {
  const now = new Date();
  const dateLabel = now.toLocaleDateString("en-US", {
    weekday: "long",
    month: "short",
    day: "numeric",
  });
  const calories = Math.round(todayTotals.calories);
  const calorieProgress = clampPercent(calories, goals.calories);
  const caloriesLeft = Math.max(0, Math.round(goals.calories - calories));
  const proteinLeft = Math.max(0, Math.round(goals.protein - todayTotals.protein));
  const companionMessage =
    entries.length === 0
      ? "Ready when you are. Snap a meal and I’ll help with the numbers."
      : proteinLeft > 0
        ? `You’re ${proteinLeft} g from your protein target. Every meal counts.`
        : "Protein target reached. Nice work listening to what your body needs.";

  const macros = [
    {
      label: "Protein",
      value: todayTotals.protein,
      goal: goals.protein,
      color: "bg-accent-600",
      track: "bg-accent-200",
    },
    {
      label: "Carbs",
      value: todayTotals.carbs,
      goal: goals.carbs,
      color: "bg-accent-2-600",
      track: "bg-accent-2-200",
    },
    {
      label: "Fat",
      value: todayTotals.fat,
      goal: goals.fat,
      color: "bg-neutral-700",
      track: "bg-neutral-300",
    },
  ];

  return (
    <div className="animate-in fade-in slide-in-from-bottom-2 flex flex-col gap-5 pb-4 duration-300">
      <header className="pt-1">
        <p className="kicker text-accent-700">{dateLabel}</p>
        <h1 className="mt-1 text-[30px] leading-tight text-ink">Good to see you, {firstName}</h1>
      </header>

      <section className="relative overflow-hidden rounded-[32px] bg-ink px-5 py-5 text-bg shadow-md">
        <div
          aria-hidden="true"
          className="absolute -right-12 -top-16 h-52 w-52 rounded-full bg-accent-2-500/25 blur-2xl"
        />
        <div className="relative flex items-center gap-4">
          <div className="min-w-0 flex-1">
            <div className="mb-3 inline-flex items-center gap-1.5 rounded-full bg-bg/10 px-2.5 py-1 text-xs font-semibold">
              <Sparkles size={13} aria-hidden="true" /> Today’s energy
            </div>
            <p className="font-display text-[42px] leading-none tabular-nums">{calories.toLocaleString("en-US")}</p>
            <p className="mt-1 text-sm text-bg/75">of {goals.calories.toLocaleString("en-US")} kcal</p>
            <p className="mt-4 text-xs font-semibold text-bg/90">
              {calories > goals.calories
                ? `${(calories - goals.calories).toLocaleString("en-US")} kcal above your guide`
                : `${caloriesLeft.toLocaleString("en-US")} kcal available`}
            </p>
          </div>

          <div className="relative grid h-28 w-28 shrink-0 place-items-center" aria-hidden="true">
            <svg viewBox="0 0 112 112" className="h-full w-full -rotate-90">
              <circle cx="56" cy="56" r="45" fill="none" stroke="rgb(245 234 216 / .16)" strokeWidth="11" />
              <circle
                cx="56"
                cy="56"
                r="45"
                fill="none"
                stroke="var(--color-accent-2-400)"
                strokeWidth="11"
                strokeLinecap="round"
                pathLength="100"
                strokeDasharray={`${calorieProgress} 100`}
              />
            </svg>
            <span className="absolute font-display text-xl tabular-nums">{Math.round(calorieProgress)}%</span>
          </div>
        </div>
      </section>

      <section aria-labelledby="mobile-macros-title">
        <div className="mb-3 flex items-end justify-between">
          <div>
            <p className="kicker text-accent-2-700">Daily balance</p>
            <h2 id="mobile-macros-title" className="mt-0.5 text-[22px] text-ink">Macros</h2>
          </div>
          <button type="button" onClick={onOpenStats} className="btn btn-ghost min-h-[44px] text-sm">
            Details <ArrowRight size={15} aria-hidden="true" />
          </button>
        </div>

        <div className="grid grid-cols-3 gap-2.5">
          {macros.map(({ label, value, goal, color, track }) => {
            const percentage = clampPercent(value, goal);
            return (
              <div key={label} className="rounded-[20px] bg-surface px-3 py-4 shadow-sm">
                <p className="text-xs font-semibold text-neutral-700">{label}</p>
                <p className="mt-1 font-display text-xl text-ink tabular-nums">
                  {Math.round(value)}<span className="ml-0.5 font-sans text-xs font-semibold">g</span>
                </p>
                <div
                  className={`mt-3 h-1.5 overflow-hidden rounded-full ${track}`}
                  role="progressbar"
                  aria-label={`${label}: ${Math.round(value)} of ${goal} grams`}
                  aria-valuemin={0}
                  aria-valuemax={goal}
                  aria-valuenow={Math.min(goal, Math.round(value))}
                >
                  <div className={`h-full rounded-full ${color}`} style={{ width: `${percentage}%` }} />
                </div>
                <p className="mt-1.5 text-[11px] font-medium text-neutral-600 tabular-nums">of {goal} g</p>
              </div>
            );
          })}
        </div>
      </section>

      <section className="relative overflow-hidden rounded-[28px] bg-accent-2-200 p-5 pr-[42%] text-accent-2-900">
        <p className="kicker">Nutri says</p>
        <h2 className="mt-1.5 text-[21px]">Small steps, real progress.</h2>
        <p className="mt-2 text-[13px] leading-relaxed text-accent-2-900/80">{companionMessage}</p>
        <img
          src={companionArt}
          alt="Nutri, your black cat nutrition companion, holding a food bowl"
          className="absolute -bottom-3 -right-3 h-[150px] w-[150px] rounded-full object-cover shadow-sm"
        />
      </section>

      <button
        type="button"
        onClick={onLogFood}
        className="group flex min-h-[76px] w-full items-center gap-4 rounded-[24px] bg-accent-solid px-5 py-4 text-left text-bg shadow-md transition-transform active:scale-[0.98]"
      >
        <span className="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-bg/15">
          <Camera size={24} strokeWidth={2.5} aria-hidden="true" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block font-display text-lg">Scan a meal</span>
          <span className="block text-xs text-bg/75">Take a photo or choose one from your gallery</span>
        </span>
        <ArrowRight size={20} className="shrink-0 transition-transform group-hover:translate-x-0.5" aria-hidden="true" />
      </button>

      <section aria-labelledby="mobile-nutrients-title" className="panel p-5">
        <div className="mb-5 flex items-end justify-between gap-4">
          <div>
            <p className="kicker text-accent-700">Beyond macros</p>
            <h2 id="mobile-nutrients-title" className="mt-0.5 text-[22px] text-ink">Nutrients</h2>
          </div>
          <span className="tag tag-neutral">Today</span>
        </div>
        <div className="flex flex-col gap-5">
          <ProgressBar label="Sodium" value={todayTotals.sodium} goal={goals.sodium} unit="mg" tone="terracotta" kind="limit" />
          <ProgressBar label="Calcium" value={todayTotals.calcium} goal={goals.calcium} unit="mg" tone="sage" />
          <ProgressBar label="Iron" value={todayTotals.iron} goal={goals.iron} unit="mg" tone="ink" />
        </div>
      </section>

      <section aria-labelledby="mobile-recent-title">
        <div className="mb-3 flex items-end justify-between">
          <div>
            <p className="kicker text-accent-2-700">Your day</p>
            <h2 id="mobile-recent-title" className="mt-0.5 text-[22px] text-ink">Recent meals</h2>
          </div>
          <span className="text-xs font-semibold text-neutral-600">{entries.length} logged</span>
        </div>

        {entries.length === 0 ? (
          <div className="rounded-[24px] border border-dashed border-neutral-500 px-5 py-7 text-center">
            <p className="font-semibold text-ink">Nothing logged yet</p>
            <p className="mx-auto mt-1 max-w-[260px] text-[13px] text-neutral-600">
              Snap your first meal when you’re ready. You can check every estimate before it is saved.
            </p>
          </div>
        ) : (
          <ul className="overflow-hidden rounded-[24px] bg-surface px-4 shadow-sm">
            {entries
              .slice()
              .reverse()
              .slice(0, 4)
              .map((entry, index) => (
                <li key={entry.id} className={`flex items-center gap-3 py-3.5 ${index > 0 ? "border-t border-divider" : ""}`}>
                  <span className="grid h-11 w-11 shrink-0 place-items-center rounded-[15px] bg-accent-2-200 font-display text-lg text-accent-2-900">
                    {entry.name.charAt(0).toUpperCase()}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-semibold text-ink">{entry.name}</span>
                    <span className="mt-0.5 flex items-center gap-1 text-xs text-neutral-600">
                      <Clock3 size={12} aria-hidden="true" />
                      {new Date(entry.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                    </span>
                  </span>
                  <span className="shrink-0 text-sm font-semibold text-ink tabular-nums">{Math.round(entry.calories)} kcal</span>
                </li>
              ))}
          </ul>
        )}
      </section>
    </div>
  );
};

export default MobileDashboardView;
