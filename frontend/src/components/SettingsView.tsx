import React, { useState } from 'react';
import { DailyGoal } from '../types/nutrition';
import { 
  User, 
  Bell, 
  Moon, 
  Target, 
  Save, 
  ShieldAlert,
  Volume2
} from 'lucide-react';

interface SettingsViewProps {
  goals: DailyGoal;
  onUpdateGoals: (goals: DailyGoal) => void;
  isDark: boolean;
  onToggleDark: () => void;
}

const SettingsView: React.FC<SettingsViewProps> = ({ goals, onUpdateGoals, isDark, onToggleDark }) => {
  const [localGoals, setLocalGoals] = useState<DailyGoal>(goals);
  const [isDirty, setIsDirty] = useState(false);
  const [showSaved, setShowSaved] = useState(false);

  const handleChange = (field: keyof DailyGoal, value: string) => {
    const numValue = parseInt(value) || 0;
    setLocalGoals(prev => ({ ...prev, [field]: numValue }));
    setIsDirty(true);
  };

  const handleSave = () => {
    onUpdateGoals(localGoals);
    setIsDirty(false);
    setShowSaved(true);
    setTimeout(() => setShowSaved(false), 2000);
  };

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 duration-500 pb-20">
      <header className="flex justify-between items-center mb-8 pt-2">
        <div>
          <h1 className="text-display-sm text-on-surface font-normal tracking-tight">Preferences</h1>
          <p className="text-on-surface-variant text-body-md mt-1">Manage your goals and app settings</p>
        </div>
      </header>

      {/* Profile Section (Mock) */}
      <section className="bg-surface-container-lowest rounded-[24px] p-6 mb-6 border border-outline-variant shadow-sm">
        <h2 className="text-title-lg font-normal text-on-surface mb-4 flex items-center gap-2">
            <User size={24} className="text-primary" />
            Profile
        </h2>
        <div className="flex items-center gap-4">
            <div className="w-16 h-16 rounded-full bg-primary text-on-primary flex items-center justify-center text-headline-sm font-medium">
                JD
            </div>
            <div className="flex-1">
                <h3 className="text-title-md font-medium text-on-surface">John Doe</h3>
                <p className="text-on-surface-variant">john.doe@example.com</p>
            </div>
            <button className="text-primary font-medium px-4 py-2 hover:bg-surface-container rounded-full transition-colors">
                Edit
            </button>
        </div>
      </section>

      {/* Goals Section */}
      <section className="bg-surface-container-lowest rounded-[24px] p-6 mb-6 border border-outline-variant shadow-sm relative overflow-hidden">
         <div className="flex justify-between items-center mb-6">
            <h2 className="text-title-lg font-normal text-on-surface flex items-center gap-2">
                <Target size={24} className="text-primary" />
                Daily Goals
            </h2>
            {isDirty && (
                <button 
                    onClick={handleSave}
                    className="bg-primary text-on-primary px-6 py-2 rounded-full font-medium flex items-center gap-2 shadow-md hover:shadow-lg transition-all animate-in fade-in"
                >
                    <Save size={18} />
                    Save Changes
                </button>
            )}
            {showSaved && (
                <span className="text-green-600 font-medium animate-in fade-in">Saved!</span>
            )}
         </div>

         <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="space-y-4">
                <div>
                    <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Daily Calories</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.calories}
                            onChange={(e) => handleChange('calories', e.target.value)}
                            className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors tabular-nums"
                        />
                        <span className="absolute right-4 top-3 text-on-surface-variant">kcal</span>
                    </div>
                </div>
                <div>
                    <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Protein Goal</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.protein}
                            onChange={(e) => handleChange('protein', e.target.value)}
                            className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors tabular-nums"
                        />
                        <span className="absolute right-4 top-3 text-on-surface-variant">g</span>
                    </div>
                </div>
                 <div>
                    <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Carbs Goal</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.carbs}
                            onChange={(e) => handleChange('carbs', e.target.value)}
                            className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors tabular-nums"
                        />
                        <span className="absolute right-4 top-3 text-on-surface-variant">g</span>
                    </div>
                </div>
            </div>
            
             <div className="space-y-4">
                <div>
                    <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Fat Goal</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.fat}
                            onChange={(e) => handleChange('fat', e.target.value)}
                            className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors tabular-nums"
                        />
                        <span className="absolute right-4 top-3 text-on-surface-variant">g</span>
                    </div>
                </div>
                 <div>
                    <label className="block text-label-lg font-medium text-on-surface-variant mb-1">Sodium Limit</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.sodium}
                            onChange={(e) => handleChange('sodium', e.target.value)}
                            className="w-full bg-surface-container border-b-2 border-outline hover:border-on-surface-variant focus:border-primary rounded-t-lg px-4 py-3 text-on-surface outline-none transition-colors tabular-nums"
                        />
                        <span className="absolute right-4 top-3 text-on-surface-variant">mg</span>
                    </div>
                </div>
            </div>
         </div>
      </section>

      {/* App Settings */}
      <section className="bg-surface-container-lowest rounded-[24px] overflow-hidden border border-outline-variant shadow-sm mb-6">
        <h2 className="text-title-lg font-normal text-on-surface p-6 pb-2 flex items-center gap-2">
             <ShieldAlert size={24} className="text-primary" />
             App Settings
        </h2>
        
        <div className="divide-y divide-outline-variant">
            <button
                onClick={onToggleDark}
                role="switch"
                aria-checked={isDark}
                className="w-full p-4 px-6 flex items-center justify-between hover:bg-surface-container transition-colors cursor-pointer"
            >
                <div className="flex items-center gap-4">
                    <Moon size={20} className="text-on-surface-variant" />
                    <span className="text-on-surface font-medium">Dark Mode</span>
                </div>
                <div className={`w-12 h-6 rounded-full relative transition-colors ${isDark ? "bg-primary-container" : "bg-outline-variant"}`}>
                    <div
                        className={`w-4 h-4 rounded-full absolute top-1 transition-all ${
                            isDark ? "bg-primary left-7" : "bg-outline left-1"
                        }`}
                    />
                </div>
            </button>
            <div className="p-4 px-6 flex items-center justify-between hover:bg-surface-container transition-colors cursor-pointer">
                <div className="flex items-center gap-4">
                    <Bell size={20} className="text-on-surface-variant" />
                    <span className="text-on-surface font-medium">Notifications</span>
                </div>
                 <div className="w-12 h-6 bg-primary-container rounded-full relative">
                    <div className="w-4 h-4 bg-primary rounded-full absolute top-1 right-1" />
                </div>
            </div>
             <div className="p-4 px-6 flex items-center justify-between hover:bg-surface-container transition-colors cursor-pointer">
                <div className="flex items-center gap-4">
                    <Volume2 size={20} className="text-on-surface-variant" />
                    <span className="text-on-surface font-medium">Sound Effects</span>
                </div>
                 <div className="w-12 h-6 bg-primary-container rounded-full relative">
                    <div className="w-4 h-4 bg-primary rounded-full absolute top-1 right-1" />
                </div>
            </div>
        </div>
      </section>

      <div className="text-center text-outline text-body-md mt-8">
        NutriMind AI v1.0.2
      </div>
    </div>
  );
};

export default SettingsView;