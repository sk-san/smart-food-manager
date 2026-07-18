import React, { useState } from 'react';
import { DailyGoal, SUGGESTED_GOALS, UserProfile } from '../types/nutrition';
import { Bell, LogOut, Moon, Volume2 } from 'lucide-react';

interface SettingsViewProps {
  goals: DailyGoal;
  onUpdateGoals: (goals: DailyGoal) => void;
  isDark: boolean;
  onToggleDark: () => void;
  /** Shows the template sign-in screen — purely presentational, no real auth. */
  onSignOut: () => void;
  /** Identity to display — the demo account, or a guest role from the bypass. */
  profile: UserProfile;
}

// Organic pill switch (visual only — interactivity comes from the row).
const Switch: React.FC<{ on: boolean }> = ({ on }) => (
  <div
    className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${
      on ? 'bg-accent-2-500' : 'bg-neutral-300'
    }`}
  >
    <div
      className={`absolute top-[3px] h-[18px] w-[18px] rounded-full bg-bg transition-all ${
        on ? 'left-[23px]' : 'left-[3px]'
      }`}
    />
  </div>
);

interface GoalFieldProps {
  label: string;
  value: number;
  onChange: (value: string) => void;
}

const GoalField: React.FC<GoalFieldProps> = ({ label, value, onChange }) => (
  <div>
    <label className="field-label">{label}</label>
    <input
      type="number"
      aria-label={label}
      className="input tabular-nums"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  </div>
);

const SettingsView: React.FC<SettingsViewProps> = ({
  goals,
  onUpdateGoals,
  isDark,
  onToggleDark,
  onSignOut,
  profile,
}) => {
  const [localGoals, setLocalGoals] = useState<DailyGoal>(goals);
  const [isDirty, setIsDirty] = useState(false);
  const [showSaved, setShowSaved] = useState(false);

  const handleChange = (field: keyof DailyGoal, value: string) => {
    const numValue = parseInt(value) || 0;
    setLocalGoals(prev => ({ ...prev, [field]: numValue }));
    setIsDirty(true);
  };

  const handleReset = () => {
    setLocalGoals(SUGGESTED_GOALS);
    setIsDirty(true);
  };

  const handleSave = () => {
    onUpdateGoals(localGoals);
    setIsDirty(false);
    setShowSaved(true);
    setTimeout(() => setShowSaved(false), 2000);
  };

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 flex flex-col gap-5 pb-20 duration-500 md:gap-6">
      <header className="pt-2">
        <h1 className="text-4xl text-ink md:text-[44px]">Your account</h1>
        <p className="mt-1.5 text-sm text-neutral-600">
          Goals feed every gauge on the dashboard — change them here.
        </p>
      </header>

      <div className="grid grid-cols-1 items-start gap-5 lg:grid-cols-[1fr_1.9fr]">
        {/* Left column: profile + app settings */}
        <div className="flex flex-col gap-4">
          <section className="flex flex-col items-center gap-2.5 rounded-card bg-surface p-7 text-center shadow-sm">
            <div className="grid h-[88px] w-[88px] place-items-center rounded-full bg-accent-300 font-display text-[32px] text-accent-900">
              {profile.initials}
            </div>
            <div>
              <div className="font-display text-[23px] text-ink">{profile.name}</div>
              <div className="mt-0.5 text-[13px] text-neutral-600">{profile.email}</div>
            </div>
            <div className="flex flex-wrap justify-center gap-1.5">
              <span className="tag tag-accent-2">5-day streak</span>
              <span className="tag tag-neutral">{profile.tag}</span>
            </div>
            <div className="mt-1.5 flex flex-wrap justify-center gap-2.5">
              <button className="btn btn-secondary">Edit profile</button>
              <button onClick={onSignOut} className="btn btn-secondary text-accent-700">
                <LogOut size={15} strokeWidth={2.5} />
                Log out
              </button>
            </div>
          </section>

          <section className="rounded-card bg-surface px-6 py-2 shadow-sm">
            <button
              onClick={onToggleDark}
              role="switch"
              aria-checked={isDark}
              className="flex w-full items-center gap-3.5 py-3.5 text-left"
            >
              <Moon size={20} strokeWidth={2.5} className="text-accent-2-700" />
              <span className="flex-1 text-sm font-semibold text-ink">Dark mode</span>
              <Switch on={isDark} />
            </button>
            <div className="flex items-center gap-3.5 border-t border-divider py-3.5">
              <Bell size={20} strokeWidth={2.5} className="text-accent-2-700" />
              <span className="flex-1 text-sm font-semibold text-ink">Reminders</span>
              <span className="text-xs text-neutral-600">8 am · 7 pm</span>
              <Switch on />
            </div>
            <div className="flex items-center gap-3.5 border-t border-divider py-3.5">
              <Volume2 size={20} strokeWidth={2.5} className="text-accent-2-700" />
              <span className="flex-1 text-sm font-semibold text-ink">Sound effects</span>
              <Switch on />
            </div>
          </section>

        </div>

        {/* Right column: daily goals */}
        <section className="rounded-card bg-surface p-7 shadow-sm md:px-8">
          <div className="flex items-baseline justify-between">
            <h3 className="text-[22px] text-ink">Daily goals</h3>
            {showSaved && (
              <span className="animate-in fade-in text-sm font-semibold text-accent-2-700">Saved!</span>
            )}
          </div>

          <div className="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-x-5">
            <GoalField
              label="Calories (kcal)"
              value={localGoals.calories}
              onChange={(v) => handleChange('calories', v)}
            />
            <GoalField
              label="Protein (g)"
              value={localGoals.protein}
              onChange={(v) => handleChange('protein', v)}
            />
            <GoalField
              label="Carbs (g)"
              value={localGoals.carbs}
              onChange={(v) => handleChange('carbs', v)}
            />
            <GoalField label="Fat (g)" value={localGoals.fat} onChange={(v) => handleChange('fat', v)} />
          </div>

          <div className="kicker mb-3 mt-6 font-semibold text-accent-2-700">Minerals</div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3 sm:gap-x-5">
            <GoalField
              label="Sodium (mg)"
              value={localGoals.sodium}
              onChange={(v) => handleChange('sodium', v)}
            />
            <GoalField
              label="Calcium (mg)"
              value={localGoals.calcium}
              onChange={(v) => handleChange('calcium', v)}
            />
            <GoalField label="Iron (mg)" value={localGoals.iron} onChange={(v) => handleChange('iron', v)} />
          </div>

          <div className="mt-7 flex flex-wrap justify-end gap-2.5">
            <button onClick={handleReset} className="btn btn-secondary">
              Reset to suggested
            </button>
            <button onClick={handleSave} disabled={!isDirty} className="btn btn-primary">
              Save goals
            </button>
          </div>
        </section>
      </div>

      <div className="mt-2 text-center text-sm text-neutral-500">Nutri · v1.0.2</div>
    </div>
  );
};

export default SettingsView;
