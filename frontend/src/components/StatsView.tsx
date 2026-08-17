import React, { useMemo } from 'react';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
  PieChart,
  Pie,
  Legend,
  ReferenceLine
} from 'recharts';
import { FoodEntry, DailyGoal } from '../types/nutrition';
import { TrendingUp, Zap, Award, Flame } from 'lucide-react';

interface StatsViewProps {
  entries: FoodEntry[];
  goals: DailyGoal;
}

const TOOLTIP_STYLE = {
  borderRadius: '16px',
  border: '1px solid var(--color-divider)',
  boxShadow: '0 3px 10px rgba(46, 43, 37, 0.16)',
  backgroundColor: 'var(--color-surface)',
  color: 'var(--color-text)',
};

const DAY_MS = 86_400_000;

const localDayKey = (date: Date) =>
  `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;

const StatsView: React.FC<StatsViewProps> = ({ entries, goals }) => {
  const weeklyData = useMemo(() => {
    const today = new Date();
    const days = Array.from({ length: 7 }, (_, index) => {
      const date = new Date(today.getFullYear(), today.getMonth(), today.getDate() - (6 - index));
      return {
        key: localDayKey(date),
        day: date.toLocaleDateString('en-US', { weekday: 'short' }),
        dateLabel: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
        calories: 0,
        protein: 0,
        fat: 0,
        carbs: 0,
        itemCount: 0,
      };
    });
    const byDay = new Map(days.map((day) => [day.key, day]));
    entries.forEach((entry) => {
      const bucket = byDay.get(localDayKey(new Date(entry.timestamp)));
      if (!bucket) return;
      bucket.calories += entry.calories;
      bucket.protein += entry.protein;
      bucket.fat += entry.fat;
      bucket.carbs += entry.carbs;
      bucket.itemCount += 1;
    });
    return days;
  }, [entries]);

  const macroDistribution = useMemo(() => {
    const totalP = weeklyData.reduce((acc, d) => acc + d.protein, 0);
    const totalC = weeklyData.reduce((acc, d) => acc + d.carbs, 0);
    const totalF = weeklyData.reduce((acc, d) => acc + d.fat, 0);

    return [
      { name: 'Protein', value: totalP, color: 'var(--color-accent-600)' },
      { name: 'Carbs', value: totalC, color: 'var(--color-accent-2-600)' },
      { name: 'Fat', value: totalF, color: 'var(--color-neutral-600)' },
    ];
  }, [weeklyData]);

  const loggedDays = weeklyData.filter((day) => day.itemCount > 0);
  const averageCalories = loggedDays.length
    ? Math.round(loggedDays.reduce((total, day) => total + day.calories, 0) / loggedDays.length)
    : 0;
  const goalDays = weeklyData.filter(
    (day) => day.itemCount > 0 && day.calories >= goals.calories * 0.8 && day.calories <= goals.calories * 1.2
  ).length;
  const weeklyEntries = entries.filter((entry) => entry.timestamp >= Date.now() - 7 * DAY_MS);
  const highestDay = weeklyData.reduce<(typeof weeklyData)[number] | null>(
    (highest, day) => (!highest || day.calories > highest.calories ? day : highest),
    null
  );
  const topProteinEntry = weeklyEntries.reduce<FoodEntry | null>(
    (top, entry) => (!top || entry.protein > top.protein ? entry : top),
    null
  );

  const weekRange = useMemo(() => {
    const end = new Date();
    const start = new Date();
    start.setDate(end.getDate() - 6);
    const fmt = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    return `${fmt(start)} – ${fmt(end)}`;
  }, []);

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-5 pb-8 duration-500 md:gap-6">
      {/* Header */}
      <header className="pt-2">
        <h1 className="text-4xl text-ink md:text-[44px]">Statistics</h1>
        <p className="mt-1.5 text-sm text-neutral-600">Weekly overview · {weekRange}</p>
      </header>

      {/* Key Metrics Row */}
      <div className="grid grid-cols-1 gap-5 md:grid-cols-3">
        <div className="panel flex items-center gap-4 p-5">
          <div className="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-accent-200 text-accent-800">
            <Flame size={21} strokeWidth={2.5} />
          </div>
          <div>
            <p className="text-[13px] text-neutral-600">Avg. calories</p>
            <div className="flex items-center gap-2">
              <span className="font-display text-[22px] text-ink tabular-nums">
                {averageCalories.toLocaleString('en-US')}
              </span>
              <span className="tag tag-neutral">logged days</span>
            </div>
          </div>
        </div>

        <div className="panel flex items-center gap-4 p-5">
          <div className="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-accent-2-200 text-accent-2-800">
            <Award size={21} strokeWidth={2.5} />
          </div>
          <div>
            <p className="text-[13px] text-neutral-600">Days near goal</p>
            <div className="flex items-center gap-2">
              <span className="font-display text-[22px] text-ink tabular-nums">{goalDays} days</span>
              <span className="tag tag-neutral">of 7</span>
            </div>
          </div>
        </div>

        <div className="panel flex items-center gap-4 p-5">
          <div className="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-accent-2-200 text-accent-2-800">
            <TrendingUp size={21} strokeWidth={2.5} />
          </div>
          <div>
            <p className="text-[13px] text-neutral-600">Items logged</p>
            <div className="flex items-center gap-2">
              <span className="font-display text-[22px] text-ink tabular-nums">{weeklyEntries.length}</span>
              <span className="tag tag-accent-2">this week</span>
            </div>
          </div>
        </div>
      </div>

      {/* Main Charts */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Weekly Calories Chart */}
        <div className="card p-6 md:px-7 lg:col-span-2">
          <div className="mb-5 flex items-center justify-between">
            <h2 className="text-[21px] text-ink">Weekly intake</h2>
            <span className="tag tag-neutral">calories</span>
          </div>

          <div className="h-[300px] w-full">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={weeklyData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--color-neutral-300)" />
                <XAxis
                  dataKey="day"
                  axisLine={false}
                  tickLine={false}
                  tick={{ fill: 'var(--color-neutral-700)', fontSize: 12 }}
                  dy={10}
                />
                <YAxis
                  axisLine={false}
                  tickLine={false}
                  tick={{ fill: 'var(--color-neutral-700)', fontSize: 12 }}
                />
                <Tooltip cursor={{ fill: 'var(--color-neutral-200)' }} contentStyle={TOOLTIP_STYLE} />
                <ReferenceLine
                  y={goals.calories}
                  stroke="var(--color-neutral-500)"
                  strokeDasharray="3 6"
                  strokeWidth={1.5}
                />
                <Bar dataKey="calories" radius={[8, 8, 8, 8]} barSize={30}>
                  {weeklyData.map((entry, index) => (
                    <Cell
                      key={`cell-${index}`}
                      fill={
                        entry.calories > goals.calories
                          ? 'var(--color-accent-600)'
                          : 'var(--color-accent-2-500)'
                      }
                    />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
          <p className="mt-2 text-[13px] text-neutral-600 tabular-nums">
            Dashed line is your {goals.calories.toLocaleString('en-US')} goal — terracotta bars went over.
          </p>
        </div>

        {/* Macro Distribution Pie Chart */}
        <div className="panel flex flex-col p-6 md:px-7">
          <h2 className="mb-5 text-[21px] text-ink">Macro split</h2>
          <div className="relative min-h-[250px] flex-1">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={macroDistribution}
                  cx="50%"
                  cy="50%"
                  innerRadius={60}
                  outerRadius={80}
                  paddingAngle={5}
                  dataKey="value"
                >
                  {macroDistribution.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} stroke="none" />
                  ))}
                </Pie>
                <Tooltip contentStyle={TOOLTIP_STYLE} />
                <Legend
                  verticalAlign="bottom"
                  height={36}
                  iconType="circle"
                  formatter={(value) => <span style={{ color: 'var(--color-neutral-700)' }}>{value}</span>}
                />
              </PieChart>
            </ResponsiveContainer>
            {/* Center Text */}
            <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center pb-8">
              <span className="font-display text-[26px] text-ink">Avg</span>
              <span className="text-[13px] text-neutral-600">distribution</span>
            </div>
          </div>
        </div>
      </div>

      {/* Top Foods / Insights List */}
      <div className="panel p-6 md:px-7">
        <h2 className="mb-4 text-[21px] text-ink">Highlights</h2>
        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between rounded-3xl bg-surface p-4">
            <div className="flex items-center gap-3">
              <div className="grid h-10 w-10 place-items-center rounded-full bg-accent-200 text-accent-800">
                <Flame size={19} strokeWidth={2.5} />
              </div>
              <div>
                <p className="text-sm font-semibold text-ink">Highest calorie day</p>
                <p className="text-[13px] text-neutral-600">
                  {highestDay?.itemCount ? `${highestDay.day}, ${highestDay.dateLabel}` : 'No entries yet'}
                </p>
              </div>
            </div>
            <span className="text-[15px] font-semibold text-ink tabular-nums">
              {Math.round(highestDay?.calories ?? 0).toLocaleString('en-US')} kcal
            </span>
          </div>

          <div className="flex items-center justify-between rounded-3xl bg-surface p-4">
            <div className="flex items-center gap-3">
              <div className="grid h-10 w-10 place-items-center rounded-full bg-accent-2-200 text-accent-2-800">
                <Zap size={19} strokeWidth={2.5} />
              </div>
              <div>
                <p className="text-sm font-semibold text-ink">Top nutrient source</p>
                <p className="text-[13px] text-neutral-600">{topProteinEntry?.name ?? 'No entries yet'}</p>
              </div>
            </div>
            <span className="text-[15px] font-semibold text-ink tabular-nums">
              {Math.round(topProteinEntry?.protein ?? 0)}g protein
            </span>
          </div>
        </div>
      </div>
    </div>
  );
};

export default StatsView;
