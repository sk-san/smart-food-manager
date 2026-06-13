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
}

const SettingsView: React.FC<SettingsViewProps> = ({ goals, onUpdateGoals }) => {
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
          <h1 className="text-4xl text-[#1d1b20] font-normal tracking-tight">Preferences</h1>
          <p className="text-[#49454f] text-sm mt-1">Manage your goals and app settings</p>
        </div>
      </header>

      {/* Profile Section (Mock) */}
      <section className="bg-white rounded-[24px] p-6 mb-6 border border-[#e7e0ec] shadow-sm">
        <h2 className="text-xl font-normal text-[#1d1b20] mb-4 flex items-center gap-2">
            <User size={24} className="text-[#6750a4]" />
            Profile
        </h2>
        <div className="flex items-center gap-4">
            <div className="w-16 h-16 rounded-full bg-[#6750a4] text-white flex items-center justify-center text-2xl font-medium">
                JD
            </div>
            <div className="flex-1">
                <h3 className="text-lg font-medium text-[#1d1b20]">John Doe</h3>
                <p className="text-[#49454f]">john.doe@example.com</p>
            </div>
            <button className="text-[#6750a4] font-medium px-4 py-2 hover:bg-[#f3f0f5] rounded-full transition-colors">
                Edit
            </button>
        </div>
      </section>

      {/* Goals Section */}
      <section className="bg-white rounded-[24px] p-6 mb-6 border border-[#e7e0ec] shadow-sm relative overflow-hidden">
         <div className="flex justify-between items-center mb-6">
            <h2 className="text-xl font-normal text-[#1d1b20] flex items-center gap-2">
                <Target size={24} className="text-[#6750a4]" />
                Daily Goals
            </h2>
            {isDirty && (
                <button 
                    onClick={handleSave}
                    className="bg-[#6750a4] text-white px-6 py-2 rounded-full font-medium flex items-center gap-2 shadow-md hover:shadow-lg transition-all animate-in fade-in"
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
                    <label className="block text-sm font-medium text-[#49454f] mb-1">Daily Calories</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.calories}
                            onChange={(e) => handleChange('calories', e.target.value)}
                            className="w-full bg-[#f3f0f5] border-b-2 border-[#79747e] hover:border-[#49454f] focus:border-[#6750a4] rounded-t-lg px-4 py-3 text-[#1d1b20] outline-none transition-colors"
                        />
                        <span className="absolute right-4 top-3 text-[#49454f]">kcal</span>
                    </div>
                </div>
                <div>
                    <label className="block text-sm font-medium text-[#49454f] mb-1">Protein Goal</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.protein}
                            onChange={(e) => handleChange('protein', e.target.value)}
                            className="w-full bg-[#f3f0f5] border-b-2 border-[#79747e] hover:border-[#49454f] focus:border-[#6750a4] rounded-t-lg px-4 py-3 text-[#1d1b20] outline-none transition-colors"
                        />
                        <span className="absolute right-4 top-3 text-[#49454f]">g</span>
                    </div>
                </div>
                 <div>
                    <label className="block text-sm font-medium text-[#49454f] mb-1">Carbs Goal</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.carbs}
                            onChange={(e) => handleChange('carbs', e.target.value)}
                            className="w-full bg-[#f3f0f5] border-b-2 border-[#79747e] hover:border-[#49454f] focus:border-[#6750a4] rounded-t-lg px-4 py-3 text-[#1d1b20] outline-none transition-colors"
                        />
                        <span className="absolute right-4 top-3 text-[#49454f]">g</span>
                    </div>
                </div>
            </div>
            
             <div className="space-y-4">
                <div>
                    <label className="block text-sm font-medium text-[#49454f] mb-1">Fat Goal</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.fat}
                            onChange={(e) => handleChange('fat', e.target.value)}
                            className="w-full bg-[#f3f0f5] border-b-2 border-[#79747e] hover:border-[#49454f] focus:border-[#6750a4] rounded-t-lg px-4 py-3 text-[#1d1b20] outline-none transition-colors"
                        />
                        <span className="absolute right-4 top-3 text-[#49454f]">g</span>
                    </div>
                </div>
                 <div>
                    <label className="block text-sm font-medium text-[#49454f] mb-1">Sodium Limit</label>
                    <div className="relative">
                        <input 
                            type="number" 
                            value={localGoals.sodium}
                            onChange={(e) => handleChange('sodium', e.target.value)}
                            className="w-full bg-[#f3f0f5] border-b-2 border-[#79747e] hover:border-[#49454f] focus:border-[#6750a4] rounded-t-lg px-4 py-3 text-[#1d1b20] outline-none transition-colors"
                        />
                        <span className="absolute right-4 top-3 text-[#49454f]">mg</span>
                    </div>
                </div>
            </div>
         </div>
      </section>

      {/* App Settings */}
      <section className="bg-white rounded-[24px] overflow-hidden border border-[#e7e0ec] shadow-sm mb-6">
        <h2 className="text-xl font-normal text-[#1d1b20] p-6 pb-2 flex items-center gap-2">
             <ShieldAlert size={24} className="text-[#6750a4]" />
             App Settings
        </h2>
        
        <div className="divide-y divide-[#e7e0ec]">
            <div className="p-4 px-6 flex items-center justify-between hover:bg-[#f3f0f5] transition-colors cursor-pointer">
                <div className="flex items-center gap-4">
                    <Moon size={20} className="text-[#49454f]" />
                    <span className="text-[#1d1b20] font-medium">Dark Mode</span>
                </div>
                <div className="w-12 h-6 bg-[#e7e0ec] rounded-full relative">
                    <div className="w-4 h-4 bg-[#79747e] rounded-full absolute top-1 left-1" />
                </div>
            </div>
            <div className="p-4 px-6 flex items-center justify-between hover:bg-[#f3f0f5] transition-colors cursor-pointer">
                <div className="flex items-center gap-4">
                    <Bell size={20} className="text-[#49454f]" />
                    <span className="text-[#1d1b20] font-medium">Notifications</span>
                </div>
                 <div className="w-12 h-6 bg-[#eaddff] rounded-full relative">
                    <div className="w-4 h-4 bg-[#6750a4] rounded-full absolute top-1 right-1" />
                </div>
            </div>
             <div className="p-4 px-6 flex items-center justify-between hover:bg-[#f3f0f5] transition-colors cursor-pointer">
                <div className="flex items-center gap-4">
                    <Volume2 size={20} className="text-[#49454f]" />
                    <span className="text-[#1d1b20] font-medium">Sound Effects</span>
                </div>
                 <div className="w-12 h-6 bg-[#eaddff] rounded-full relative">
                    <div className="w-4 h-4 bg-[#6750a4] rounded-full absolute top-1 right-1" />
                </div>
            </div>
        </div>
      </section>

      <div className="text-center text-[#79747e] text-sm mt-8">
        NutriMind AI v1.0.2
      </div>
    </div>
  );
};

export default SettingsView;