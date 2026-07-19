import React from 'react';
import { FoodEntry, DailyGoal, NutritionData } from '../types/nutrition';

interface DayPlateBoardProps {
  todayTotals: NutritionData;
  goals: DailyGoal;
  entries: FoodEntry[];
  onHoverStart: () => void;
  onHoverEnd: () => void;
}

// Plate geometry (design 1b): meals orbit a 24-hour ring at the time they
// were eaten, sized by calories, on a 340x340 canvas.
const CENTER = 170;
const RING_RADIUS = 140;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

/** Point on the clock face: midnight at the top, hours running clockwise. */
const clockPoint = (hours: number, radius: number) => {
  const angle = (hours / 24) * 2 * Math.PI;
  return {
    x: CENTER + radius * Math.sin(angle),
    y: CENTER - radius * Math.cos(angle),
  };
};

const bubbleRadius = (kcal: number) => Math.min(30, Math.max(16, 12 + kcal * 0.035));

/** Short lowercase label for a meal bubble ("salad", "oatmeal"): the last
 * word of the name, or the first that fits if the last is too wide. */
const bubbleLabel = (name: string) => {
  const words = name.toLowerCase().split(/[^a-z0-9]+/).filter(Boolean);
  const last = words[words.length - 1] ?? '';
  const fitting = last.length <= 8 ? last : words.find((w) => w.length <= 8);
  return fitting ?? last.slice(0, 8);
};

const HOUR_LABELS: { text: string; x: number; y: number }[] = [
  { text: '12a', x: CENTER, y: 26 },
  { text: '6a', x: 322, y: 174 },
  { text: '12p', x: CENTER, y: 330 },
  { text: '6p', x: 18, y: 174 },
];

const DayPlateBoard: React.FC<DayPlateBoardProps> = ({
  todayTotals,
  goals,
  entries,
  onHoverStart,
  onHoverEnd,
}) => {
  const calories = Math.round(todayTotals.calories);
  const remaining = goals.calories - calories;
  const progress = goals.calories > 0 ? Math.min(1, calories / goals.calories) : 0;

  const now = new Date();
  const nowHours = now.getHours() + now.getMinutes() / 60;
  const nowPos = clockPoint(nowHours, RING_RADIUS);

  const macros = [
    {
      label: 'Protein',
      value: todayTotals.protein,
      goal: goals.protein,
      kickerClass: 'text-accent-700',
      barColor: 'var(--color-accent-500)',
    },
    {
      label: 'Carbs',
      value: todayTotals.carbs,
      goal: goals.carbs,
      kickerClass: 'text-accent-2-700',
      barColor: 'var(--color-accent-2-500)',
    },
    {
      label: 'Fat',
      value: todayTotals.fat,
      goal: goals.fat,
      kickerClass: 'text-neutral-700',
      barColor: 'var(--color-accent-700)',
    },
  ];

  return (
    <div className="mx-auto flex w-full max-w-[430px] flex-col">
      <svg
        viewBox="0 0 340 340"
        className="h-auto w-full"
        role="img"
        aria-label={`Today as a plate: ${calories} of ${goals.calories} kcal, meals placed at the time they were eaten`}
        onMouseEnter={onHoverStart}
        onMouseLeave={onHoverEnd}
      >
        {/* Plate + dashed inner rim */}
        <circle cx={CENTER} cy={CENTER} r="106" fill="var(--color-surface)" />
        <circle
          cx={CENTER}
          cy={CENTER}
          r="86"
          fill="none"
          strokeWidth="1.5"
          strokeDasharray="2 6"
          className="stroke-neutral-300"
        />

        {/* 24-hour ring with the calorie-goal progress arc */}
        <circle
          cx={CENTER}
          cy={CENTER}
          r={RING_RADIUS}
          fill="none"
          strokeWidth="13"
          opacity="0.55"
          className="stroke-neutral-300"
        />
        <circle
          cx={CENTER}
          cy={CENTER}
          r={RING_RADIUS}
          fill="none"
          stroke="var(--color-accent-500)"
          strokeWidth="13"
          strokeLinecap="round"
          strokeDasharray={`${progress * RING_CIRCUMFERENCE} ${RING_CIRCUMFERENCE}`}
          transform={`rotate(-90 ${CENTER} ${CENTER})`}
          style={{ transition: 'stroke-dasharray 1s ease-in-out' }}
        />

        {HOUR_LABELS.map(({ text, x, y }) => (
          <text
            key={text}
            x={x}
            y={y}
            textAnchor="middle"
            fontSize="10"
            fontFamily="Figtree"
            className="fill-neutral-600"
          >
            {text}
          </text>
        ))}

        {/* Center readout */}
        <text
          x={CENTER}
          y="160"
          textAnchor="middle"
          fontFamily="Caprasimo"
          fontSize="52"
          className="fill-ink tabular-nums"
        >
          {calories.toLocaleString('en-US')}
        </text>
        <text
          x={CENTER}
          y="184"
          textAnchor="middle"
          fontSize="12.5"
          fontFamily="Figtree"
          className="fill-neutral-600 tabular-nums"
        >
          of {goals.calories.toLocaleString('en-US')} kcal
        </text>
        <text
          x={CENTER}
          y="205"
          textAnchor="middle"
          fontSize="11"
          fontFamily="Figtree"
          fontWeight="600"
          fill="var(--color-accent-700)"
          className="tabular-nums"
        >
          {entries.length === 0
            ? 'an empty plate — log a meal'
            : remaining >= 0
              ? `${remaining.toLocaleString('en-US')} left for dinner`
              : `over by ${Math.abs(remaining).toLocaleString('en-US')}`}
        </text>

        {/* Meals, placed at the time they were eaten and sized by calories */}
        {entries.map((entry, index) => {
          const at = new Date(entry.timestamp);
          const pos = clockPoint(at.getHours() + at.getMinutes() / 60, RING_RADIUS);
          const r = bubbleRadius(entry.calories);
          const palette =
            index % 2 === 0
              ? {
                  fill: 'var(--color-accent-2-200)',
                  stroke: 'var(--color-accent-2-500)',
                  value: 'var(--color-accent-2-900)',
                  label: 'var(--color-accent-2-800)',
                }
              : {
                  fill: 'var(--color-accent-200)',
                  stroke: 'var(--color-accent-500)',
                  value: 'var(--color-accent-900)',
                  label: 'var(--color-accent-800)',
                };
          return (
            <g key={entry.id}>
              <circle cx={pos.x} cy={pos.y} r={r} fill={palette.fill} stroke={palette.stroke} strokeWidth="2.5" />
              <text
                x={pos.x}
                y={pos.y - 3}
                textAnchor="middle"
                fontSize="10.5"
                fontWeight="700"
                fontFamily="Figtree"
                fill={palette.value}
                className="tabular-nums"
              >
                {Math.round(entry.calories)}
              </text>
              <text
                x={pos.x}
                y={pos.y + 10}
                textAnchor="middle"
                fontSize="8.5"
                fontFamily="Figtree"
                fill={palette.label}
              >
                {bubbleLabel(entry.name)}
              </text>
            </g>
          );
        })}

        {/* "Now" marker on the ring */}
        <circle cx={nowPos.x} cy={nowPos.y} r="6" fill="var(--color-bg)" stroke="var(--color-accent-700)" strokeWidth="3" />
        <text
          x={nowPos.x}
          y={nowPos.y - 14}
          textAnchor="middle"
          fontSize="9.5"
          fontWeight="600"
          fontFamily="Figtree"
          fill="var(--color-accent-700)"
        >
          now
        </text>
      </svg>

      {/* Macro tiles */}
      <div className="mt-1 flex gap-2.5">
        {macros.map(({ label, value, goal, kickerClass, barColor }) => {
          const pct = goal > 0 ? Math.min(100, (value / goal) * 100) : 0;
          return (
            <div key={label} className="flex-1 rounded-[20px] bg-surface px-3.5 py-3 shadow-sm">
              <div className={`kicker ${kickerClass}`}>{label}</div>
              <div className="my-1 font-display text-[19px] text-ink tabular-nums">{Math.round(value)} g</div>
              <div className="h-1.5 rounded-full bg-neutral-200">
                <div
                  className="h-full rounded-full"
                  style={{ width: `${pct}%`, background: barColor, transition: 'width 1s ease-in-out' }}
                />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default DayPlateBoard;
