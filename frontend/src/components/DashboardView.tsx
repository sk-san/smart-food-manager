import React from 'react';
import { 
  Flame, 
  Beef, 
  Wheat, 
  Droplet,
  TrendingUp,
  Zap,
  Bone,
  Dumbbell,
  ChevronRight,
  MoreVertical
} from 'lucide-react';
import { AreaChart, Area, ResponsiveContainer } from 'recharts';
import NutritionCard from './NutritionCard';
import { FoodEntry, DailyGoal, NutritionData } from '../types/nutrition';

interface DashboardViewProps {
  todayTotals: NutritionData;
  goals: DailyGoal;
  entries: FoodEntry[];
  onHoverStart: () => void;
  onHoverEnd: () => void;
}

const DashboardView: React.FC<DashboardViewProps> = ({ 
  todayTotals, 
  goals, 
  entries, 
  onHoverStart, 
  onHoverEnd 
}) => {
  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
      {/* Header */}
      <header className="flex justify-between items-center mb-8 pt-2">
        <div>
          <h1 className="text-4xl text-[#1d1b20] font-normal tracking-tight">Today</h1>
          <p className="text-[#49454f] text-sm mt-1">
            {new Date().toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })}
          </p>
        </div>
        <div className="w-10 h-10 rounded-full bg-[#6750a4] text-white flex items-center justify-center font-bold text-sm shadow-md">
          JD
        </div>
      </header>

      {/* Hero Section */}
      <section className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-8">
          {/* Calories Hero */}
          <div 
              className="lg:col-span-2 bg-[#21005d] text-white rounded-[32px] p-8 relative overflow-hidden flex flex-col justify-between shadow-lg transition-transform hover:scale-[1.01]"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
          >
              <div className="z-10 flex justify-between items-start">
                  <div>
                      <h2 className="text-3xl font-normal opacity-90">Calories</h2>
                      <p className="text-white/70 mt-1">Daily Limit: {goals.calories}</p>
                  </div>
                  <div className="bg-white/10 p-3 rounded-full backdrop-blur-md">
                      <Flame className="text-[#d0bcff]" size={32} />
                  </div>
              </div>
              
              <div className="z-10 mt-8 flex items-end gap-2">
                  <span className="text-7xl font-bold tracking-tighter">{Math.round(todayTotals.calories)}</span>
                  <span className="text-2xl opacity-60 mb-2">kcal</span>
              </div>

              <div className="mt-6 z-10">
                  <div className="flex justify-between text-sm opacity-80 mb-2">
                      <span>Progress</span>
                      <span>{Math.round((todayTotals.calories / goals.calories) * 100)}%</span>
                  </div>
                  <div className="w-full bg-white/20 h-3 rounded-full overflow-hidden">
                      <div 
                          className="h-full bg-[#d0bcff] rounded-full transition-all duration-1000 ease-out" 
                          style={{ width: `${Math.min(100, (todayTotals.calories / goals.calories) * 100)}%` }} 
                      />
                  </div>
              </div>

              {/* Decorative blobs */}
              <div className="absolute -top-20 -right-20 w-80 h-80 bg-[#6750a4] rounded-full blur-3xl opacity-50" />
              <div className="absolute -bottom-20 -left-20 w-60 h-60 bg-[#EADDFF] rounded-full blur-3xl opacity-20" />
          </div>

          {/* Quick Chart */}
          <div 
              className="bg-[#eaddff] rounded-[32px] p-6 flex flex-col shadow-sm transition-transform hover:scale-[1.01]"
              onMouseEnter={onHoverStart}
              onMouseLeave={onHoverEnd}
          >
              <div className="flex justify-between items-center mb-4">
                  <h3 className="text-[#21005d] text-lg font-medium">Activity</h3>
                  <TrendingUp size={20} className="text-[#21005d]" />
              </div>
              <div className="flex-1 min-h-[160px]">
                  <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={[
                          { name: 'M', val: 1800 },
                          { name: 'T', val: 2100 },
                          { name: 'W', val: 1950 },
                          { name: 'T', val: 2200 },
                          { name: 'F', val: todayTotals.calories },
                      ]}>
                           <defs>
                              <linearGradient id="colorVal" x1="0" y1="0" x2="0" y2="1">
                              <stop offset="5%" stopColor="#6750a4" stopOpacity={0.3}/>
                              <stop offset="95%" stopColor="#6750a4" stopOpacity={0}/>
                              </linearGradient>
                          </defs>
                          <Area type="monotone" dataKey="val" stroke="#6750a4" strokeWidth={3} fillOpacity={1} fill="url(#colorVal)" />
                      </AreaChart>
                  </ResponsiveContainer>
              </div>
              <p className="text-[#49454f] text-sm mt-2 text-center">Calories over last 5 days</p>
          </div>
      </section>

      {/* Macros Grid */}
      <section className="mb-8">
          <h3 className="text-xl text-[#1d1b20] font-normal mb-4 px-1">Macros</h3>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <NutritionCard 
                  label="Protein" 
                  value={todayTotals.protein} 
                  goal={goals.protein} 
                  unit="g"
                  color="bg-rose-400"
                  icon={<Beef size={20} />}
                  onMouseEnter={onHoverStart}
                  onMouseLeave={onHoverEnd}
              />
              <NutritionCard 
                  label="Carbs" 
                  value={todayTotals.carbs} 
                  goal={goals.carbs} 
                  unit="g"
                  color="bg-amber-400"
                  icon={<Wheat size={20} />}
                  onMouseEnter={onHoverStart}
                  onMouseLeave={onHoverEnd}
              />
              <NutritionCard 
                  label="Fat" 
                  value={todayTotals.fat} 
                  goal={goals.fat} 
                  unit="g"
                  color="bg-teal-400"
                  icon={<Droplet size={20} />}
                  onMouseEnter={onHoverStart}
                  onMouseLeave={onHoverEnd}
              />
          </div>
      </section>

      {/* Minerals Grid */}
      <section className="mb-8">
          <h3 className="text-xl text-[#1d1b20] font-normal mb-4 px-1">Minerals</h3>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <NutritionCard 
                  label="Sodium" 
                  value={todayTotals.sodium} 
                  goal={goals.sodium} 
                  unit="mg"
                  color="bg-cyan-400"
                  icon={<Zap size={20} />}
                  onMouseEnter={onHoverStart}
                  onMouseLeave={onHoverEnd}
              />
              <NutritionCard 
                  label="Calcium" 
                  value={todayTotals.calcium} 
                  goal={goals.calcium} 
                  unit="mg"
                  color="bg-slate-300"
                  icon={<Bone size={20} />}
                  onMouseEnter={onHoverStart}
                  onMouseLeave={onHoverEnd}
              />
              <NutritionCard 
                  label="Iron" 
                  value={todayTotals.iron} 
                  goal={goals.iron} 
                  unit="mg"
                  color="bg-red-300"
                  icon={<Dumbbell size={20} />}
                  onMouseEnter={onHoverStart}
                  onMouseLeave={onHoverEnd}
              />
          </div>
      </section>

      {/* Recent Entries */}
      <section 
          className="bg-white rounded-[32px] p-6 md:p-8 shadow-sm border border-[#e7e0ec] transition-transform hover:scale-[1.005]"
          onMouseEnter={onHoverStart}
          onMouseLeave={onHoverEnd}
      >
          <div className="flex justify-between items-center mb-6">
              <h3 className="text-xl text-[#1d1b20] font-normal">Recent Logs</h3>
              <button className="text-[#6750a4] font-medium text-sm flex items-center hover:bg-[#eaddff]/20 px-3 py-1 rounded-full transition-colors">
                  View All <ChevronRight size={16} />
              </button>
          </div>

          <div className="space-y-1">
              {entries.slice().reverse().map((entry) => (
                  <div key={entry.id} className="group flex items-center justify-between p-4 hover:bg-[#f3f0f5] rounded-2xl transition-colors cursor-pointer border border-transparent hover:border-[#e7e0ec]">
                      <div className="flex items-center gap-4">
                          <div className="w-12 h-12 rounded-full bg-[#eaddff] text-[#21005d] flex items-center justify-center font-medium text-lg">
                              {entry.name.charAt(0).toUpperCase()}
                          </div>
                          <div>
                              <h4 className="text-[#1d1b20] font-medium text-lg leading-tight">{entry.name}</h4>
                              <p className="text-[#49454f] text-sm mt-0.5">
                                  {new Date(entry.timestamp).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})}
                              </p>
                          </div>
                      </div>
                      <div className="flex items-center gap-6">
                          <div className="text-right hidden sm:block">
                              <span className="block font-bold text-[#1d1b20]">{entry.calories} kcal</span>
                              <span className="text-xs text-[#49454f]">
                                  P:{Math.round(entry.protein)} C:{Math.round(entry.carbs)} F:{Math.round(entry.fat)}
                              </span>
                          </div>
                          <button className="text-[#49454f] opacity-0 group-hover:opacity-100 transition-opacity p-2 hover:bg-[#e7e0ec] rounded-full">
                              <MoreVertical size={20} />
                          </button>
                      </div>
                  </div>
              ))}
          </div>
      </section>
    </div>
  );
};

export default DashboardView;
