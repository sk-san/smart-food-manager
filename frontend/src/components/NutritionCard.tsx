import React from "react";

interface NutritionCardProps {
  label: string;
  value: number;
  goal: number;
  unit: string;
  color: string;
  icon: React.ReactNode;
  onMouseEnter?: () => void;
  onMouseLeave?: () => void;
}

const NutritionCard: React.FC<NutritionCardProps> = ({
  label,
  value,
  goal,
  unit,
  color,
  icon,
  onMouseEnter,
  onMouseLeave,
}) => {
  const percentage = Math.min(100, Math.max(0, (value / goal) * 100));

  return (
    <div
      className="bg-surface-container rounded-[24px] p-4 flex flex-col relative overflow-hidden group hover:bg-surface-container-high transition-colors duration-300"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <div className="flex justify-between items-start mb-2 z-10">
        <div className={`p-3 rounded-2xl ${color} bg-opacity-20 text-gray-900 dark:text-gray-100`}>{icon}</div>
        <div className="text-right">
          <span className="block text-headline-sm font-bold text-on-surface tabular-nums">{Math.round(value)}</span>
          <span className="text-label-md text-on-surface-variant font-medium tabular-nums">
            / {goal} {unit}
          </span>
        </div>
      </div>

      <div className="mt-auto z-10">
        <h3 className="text-label-lg font-medium text-on-surface-variant">{label}</h3>
        <div className="w-full bg-outline-variant h-2 rounded-full mt-2 overflow-hidden">
          <div
            className={`h-full rounded-full ${color}`}
            style={{ width: `${percentage}%`, transition: "width 1s ease-in-out" }}
          />
        </div>
      </div>

      {/* Subtle decorative circle background */}
      <div
        className={`absolute -bottom-8 -right-8 w-24 h-24 rounded-full ${color} opacity-10 pointer-events-none group-hover:scale-110 transition-transform duration-500`}
      />
    </div>
  );
};

export default NutritionCard;
