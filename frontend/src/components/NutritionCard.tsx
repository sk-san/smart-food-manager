import React from "react";

interface NutritionCardProps {
  label: string;
  value: number;
  goal: number;
  unit: string;
  /** Stroke color for the progress ring, e.g. "var(--color-accent-500)". */
  ringColor: string;
  /** Tailwind text color class for the kicker label. */
  kickerClass: string;
  onMouseEnter?: () => void;
  onMouseLeave?: () => void;
}

const RADIUS = 30;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

// Organic ring gauge card (design 1a): a round progress dial with the value
// and goal beside it.
const NutritionCard: React.FC<NutritionCardProps> = ({
  label,
  value,
  goal,
  unit,
  ringColor,
  kickerClass,
  onMouseEnter,
  onMouseLeave,
}) => {
  const percentage = goal > 0 ? Math.min(100, Math.max(0, (value / goal) * 100)) : 0;

  return (
    <div
      className="flex items-center gap-4 rounded-card bg-surface p-5 md:gap-5 md:px-6"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <svg width="76" height="76" viewBox="0 0 76 76" className="shrink-0" aria-hidden="true">
        <circle cx="38" cy="38" r={RADIUS} fill="none" strokeWidth="9" className="stroke-neutral-200" />
        <circle
          cx="38"
          cy="38"
          r={RADIUS}
          fill="none"
          strokeWidth="9"
          strokeLinecap="round"
          stroke={ringColor}
          strokeDasharray={`${(percentage / 100) * CIRCUMFERENCE} ${CIRCUMFERENCE}`}
          transform="rotate(-90 38 38)"
          style={{ transition: "stroke-dasharray 1s ease-in-out" }}
        />
        <text
          x="38"
          y="43"
          textAnchor="middle"
          fontSize="15"
          fontFamily="Caprasimo"
          className="fill-ink tabular-nums"
        >
          {Math.round(percentage)}%
        </text>
      </svg>
      <div>
        <span className={`kicker ${kickerClass}`}>{label}</span>
        <div className="mt-0.5 font-display text-2xl text-ink tabular-nums">
          {Math.round(value)} {unit}
        </div>
        <div className="text-xs text-neutral-600 tabular-nums">
          of {goal.toLocaleString("en-US")} {unit}
        </div>
      </div>
    </div>
  );
};

export default NutritionCard;
