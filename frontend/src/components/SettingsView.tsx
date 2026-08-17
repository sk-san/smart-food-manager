import React, { useEffect, useState } from 'react';
import { DailyGoal, DEMO_ACCOUNT_STATS, SUGGESTED_GOALS, UserProfile } from '../types/nutrition';
import { BookOpen, Bell, CircleDashed, LogOut, Moon, Rows3, Volume2 } from 'lucide-react';
import { DASHBOARD_LAYOUTS, DashboardLayout } from '../preferences';

interface SettingsViewProps {
  goals: DailyGoal;
  onUpdateGoals: (goals: DailyGoal) => void | Promise<void>;
  isDark: boolean;
  onToggleDark: () => void;
  /** Clears the authenticated session and returns to sign-in. */
  onSignOut: () => void;
  /** Identity to display — the demo account, or a guest role from the bypass. */
  profile: UserProfile;
  dashboardLayout: DashboardLayout;
  onDashboardLayoutChange: (layout: DashboardLayout) => void;
}

// Organic pill switch (visual only — interactivity comes from the row).
const Switch: React.FC<{ on: boolean }> = ({ on }) => (
  <div
    className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${
      on ? 'bg-accent-2-600' : 'bg-neutral-400'
    }`}
  >
    <div
      className={`absolute top-[3px] h-[18px] w-[18px] rounded-full bg-bg transition-all ${
        on ? 'left-[23px]' : 'left-[3px]'
      }`}
    />
  </div>
);

const LAYOUT_ICONS: Record<DashboardLayout, typeof Rows3> = {
  ledger: Rows3,
  plate: CircleDashed,
  almanac: BookOpen,
};

interface GoalFieldProps {
  label: string;
  value: number;
  onChange: (value: string) => void;
}

const GoalField: React.FC<GoalFieldProps> = ({ label, value, onChange }) => {
  // Keep the user's in-progress text separate from the numeric goal. This lets
  // a keyboard user clear the field before typing a replacement value instead
  // of React immediately forcing the empty field back to zero.
  const [draft, setDraft] = useState(String(value));

  useEffect(() => {
    setDraft(String(value));
  }, [value]);

  return (
    <div>
      <label className="field-label">{label}</label>
      <input
        type="number"
        min="0"
        inputMode="numeric"
        aria-label={label}
        className="input tabular-nums"
        value={draft}
        onChange={(event) => {
          const nextValue = event.target.value;
          setDraft(nextValue);
          if (nextValue !== '') onChange(nextValue);
        }}
        onBlur={() => {
          if (draft === '') setDraft(String(value));
        }}
      />
    </div>
  );
};

const SettingsView: React.FC<SettingsViewProps> = ({
  goals,
  onUpdateGoals,
  isDark,
  onToggleDark,
  onSignOut,
  profile,
  dashboardLayout,
  onDashboardLayoutChange,
}) => {
  const [localGoals, setLocalGoals] = useState<DailyGoal>(goals);
  const [isDirty, setIsDirty] = useState(false);
  const [showSaved, setShowSaved] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!isDirty) setLocalGoals(goals);
  }, [goals, isDirty]);

  const handleChange = (field: keyof DailyGoal, value: string) => {
    const numValue = parseInt(value) || 0;
    setLocalGoals(prev => ({ ...prev, [field]: numValue }));
    setIsDirty(true);
  };

  const handleReset = () => {
    setLocalGoals(SUGGESTED_GOALS);
    setIsDirty(true);
  };

  const handleSave = async () => {
    setIsSaving(true);
    setSaveError(null);
    try {
      await onUpdateGoals(localGoals);
      setIsDirty(false);
      setShowSaved(true);
      setTimeout(() => setShowSaved(false), 2000);
    } catch (error) {
      console.error(error);
      setSaveError('Goals could not be saved. Check your connection and try again.');
    } finally {
      setIsSaving(false);
    }
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
        {/* Left column: profile + app settings. All supporting material to the
            goals form, so all of it stays on open panels. */}
        <div className="flex flex-col gap-4">
          <section className="panel flex flex-col items-center gap-2.5 p-7 text-center">
            <div className="grid h-[88px] w-[88px] place-items-center rounded-full bg-accent-300 font-display text-[32px] text-accent-900">
              {profile.initials}
            </div>
            <div>
              <h2 className="font-display text-[23px] text-ink">{profile.name}</h2>
              <div className="mt-0.5 text-[13px] text-neutral-600">{profile.email}</div>
            </div>
            <div className="flex flex-wrap justify-center gap-1.5">
              <span className="tag tag-accent-2 tabular-nums">{DEMO_ACCOUNT_STATS.currentStreak}-day streak</span>
              <span className="tag tag-neutral">{profile.tag}</span>
            </div>
            <button onClick={onSignOut} className="btn btn-secondary mt-1.5 text-accent-700">
              <LogOut size={15} strokeWidth={2.5} />
              Log out
            </button>
          </section>

          {/* Week at a glance (design 2b) — demo aggregates until entries persist. */}
          <section className="grid grid-cols-3 gap-2.5" aria-label="Account statistics">
            {[
              { value: DEMO_ACCOUNT_STATS.daysLogged.toLocaleString('en-US'), caption: 'days logged' },
              { value: DEMO_ACCOUNT_STATS.avgCalories.toLocaleString('en-US'), caption: 'avg kcal / day' },
              { value: DEMO_ACCOUNT_STATS.bestStreak.toLocaleString('en-US'), caption: 'best streak' },
            ].map(({ value, caption }) => (
              <div key={caption} className="panel px-2 py-3.5 text-center">
                <div className="font-display text-[21px] text-ink tabular-nums">{value}</div>
                <div className="text-[13px] leading-snug text-neutral-600">{caption}</div>
              </div>
            ))}
          </section>

          {/* Today's layout. Ledger, Plate and Almanac are three drawings of the
              same day, so they belong here as a preference rather than as a
              switcher competing for attention on the dashboard itself. */}
          <section className="panel px-6 py-5">
            <fieldset>
              <legend className="kicker mb-3 text-accent-2-700">Today&apos;s layout</legend>
              <div className="flex flex-col gap-1">
                {DASHBOARD_LAYOUTS.map(({ id, label, description }) => {
                  const Icon = LAYOUT_ICONS[id];
                  const selected = dashboardLayout === id;
                  return (
                    <label
                      key={id}
                      className={`flex cursor-pointer items-start gap-3 rounded-2xl px-3 py-2.5 transition-colors ${
                        selected ? 'bg-accent-2-100' : 'hover:bg-neutral-100'
                      }`}
                    >
                      <input
                        type="radio"
                        name="dashboard-layout"
                        value={id}
                        checked={selected}
                        onChange={() => onDashboardLayoutChange(id)}
                        className="mt-1 h-4 w-4 shrink-0 accent-[var(--color-accent-solid)]"
                      />
                      <Icon
                        size={18}
                        strokeWidth={2.5}
                        aria-hidden="true"
                        className={`mt-0.5 shrink-0 ${selected ? 'text-accent-2-800' : 'text-neutral-600'}`}
                      />
                      <span>
                        <span className="block text-sm font-semibold text-ink">{label}</span>
                        <span className="block text-[13px] leading-snug text-neutral-600">{description}</span>
                      </span>
                    </label>
                  );
                })}
              </div>
            </fieldset>
          </section>

          <section className="panel px-6 py-2">
            <button
              onClick={onToggleDark}
              role="switch"
              aria-checked={isDark}
              className="flex w-full items-center gap-3.5 py-3.5 text-left"
            >
              <Moon size={20} strokeWidth={2.5} className="text-accent-2-700" aria-hidden="true" />
              <span className="flex-1 text-sm font-semibold text-ink">Dark mode</span>
              <Switch on={isDark} />
            </button>

            {/* Designed, not built. Shown as plain rows rather than switches so
                nothing here looks like it can be turned on. */}
            <div className="flex items-center gap-3.5 border-t border-divider py-3.5">
              <Bell size={20} strokeWidth={2.5} className="text-neutral-600" aria-hidden="true" />
              <span className="flex-1 text-sm font-semibold text-neutral-700">Reminders</span>
              <span className="tag tag-neutral">Not built yet</span>
            </div>
            <div className="flex items-center gap-3.5 border-t border-divider py-3.5">
              <Volume2 size={20} strokeWidth={2.5} className="text-neutral-600" aria-hidden="true" />
              <span className="flex-1 text-sm font-semibold text-neutral-700">Sound effects</span>
              <span className="tag tag-neutral">Not built yet</span>
            </div>
          </section>
        </div>

        {/* Right column: the actual task of this screen — the one elevated card. */}
        <section className="card p-7 md:px-8">
          <div className="flex items-baseline justify-between">
            <h2 className="text-[22px] text-ink">Daily goals</h2>
            <span role="status" aria-live="polite" className="text-sm font-semibold text-accent-2-700">
              {showSaved ? 'Saved!' : ''}
            </span>
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

          <div className="kicker mb-3 mt-6 text-accent-2-700">Minerals</div>
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
            {saveError && <p role="alert" className="mr-auto text-sm font-semibold text-accent-800">{saveError}</p>}
            <button onClick={handleReset} className="btn btn-secondary">
              Reset to suggested
            </button>
            <button onClick={handleSave} disabled={!isDirty || isSaving} className="btn btn-primary">
              {isSaving ? 'Saving…' : 'Save goals'}
            </button>
          </div>
        </section>
      </div>

      <div className="mt-2 text-center text-sm text-neutral-600">Nutri · v1.0.2</div>
    </div>
  );
};

export default SettingsView;
