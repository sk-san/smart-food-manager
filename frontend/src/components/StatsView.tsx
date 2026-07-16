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
  Legend 
} from 'recharts';
import { FoodEntry, DailyGoal } from '../types/nutrition';
import { TrendingUp, Zap, Award, Flame, ArrowUpRight, ArrowDownRight } from 'lucide-react';

interface StatsViewProps {
  entries: FoodEntry[];
  goals: DailyGoal;
}

const StatsView: React.FC<StatsViewProps> = ({ entries, goals }) => {
  // Generate mock data mixed with real data for better visualization
  const weeklyData = useMemo(() => {
    // In a real app, we would aggregate real entries. 
    // Here we will use mock data for previous days and real data for today/recent.
    return [
      { day: 'Mon', calories: 2150, protein: 140, fat: 65, carbs: 220 },
      { day: 'Tue', calories: 1850, protein: 125, fat: 55, carbs: 190 },
      { day: 'Wed', calories: 2400, protein: 160, fat: 80, carbs: 260 },
      { day: 'Thu', calories: 1950, protein: 135, fat: 60, carbs: 210 },
      { day: 'Fri', calories: 2250, protein: 150, fat: 70, carbs: 240 },
      { day: 'Sat', calories: 2600, protein: 110, fat: 95, carbs: 300 },
      { day: 'Sun', calories: entries.reduce((acc, e) => acc + e.calories, 0) || 1900, protein: 130, fat: 60, carbs: 200 },
    ];
  }, [entries]);

  const macroDistribution = useMemo(() => {
    const totalP = weeklyData.reduce((acc, d) => acc + d.protein, 0);
    const totalC = weeklyData.reduce((acc, d) => acc + d.carbs, 0);
    const totalF = weeklyData.reduce((acc, d) => acc + d.fat, 0);
    
    return [
      { name: 'Protein', value: totalP, color: '#f43f5e' }, // rose-500
      { name: 'Carbs', value: totalC, color: '#fbbf24' },   // amber-400
      { name: 'Fat', value: totalF, color: '#2dd4bf' },     // teal-400
    ];
  }, [weeklyData]);

  const averageCalories = Math.round(weeklyData.reduce((acc, d) => acc + d.calories, 0) / weeklyData.length);
  
  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 duration-500 pb-8">
      {/* Header */}
      <header className="flex justify-between items-center mb-8 pt-2">
        <div>
          <h1 className="text-display-sm text-on-surface font-normal tracking-tight">Statistics</h1>
          <p className="text-on-surface-variant text-body-md mt-1">
             Weekly Overview • Oct 23 - Oct 30
          </p>
        </div>
        <div className="w-10 h-10 rounded-full bg-primary text-on-primary flex items-center justify-center font-bold text-sm shadow-md">
          JD
        </div>
      </header>

      {/* Key Metrics Row */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
        <div className="bg-surface-container-lowest p-5 rounded-[24px] border border-outline-variant shadow-sm flex items-center gap-4">
           <div className="w-12 h-12 rounded-full bg-primary-container flex items-center justify-center text-on-primary-container">
             <Flame size={24} />
           </div>
           <div>
             <p className="text-body-md text-on-surface-variant">Avg. Calories</p>
             <div className="flex items-center gap-2">
                <span className="text-headline-sm font-bold text-on-surface tabular-nums">{averageCalories}</span>
                <span className="text-xs text-green-600 bg-green-100 px-1.5 py-0.5 rounded-md flex items-center">
                    <ArrowUpRight size={12} className="mr-0.5" /> 2%
                </span>
             </div>
           </div>
        </div>

        <div className="bg-surface-container-lowest p-5 rounded-[24px] border border-outline-variant shadow-sm flex items-center gap-4">
           <div className="w-12 h-12 rounded-full bg-primary-container flex items-center justify-center text-on-primary-container">
             <Award size={24} />
           </div>
           <div>
             <p className="text-body-md text-on-surface-variant">Goal Streak</p>
             <div className="flex items-center gap-2">
                <span className="text-headline-sm font-bold text-on-surface tabular-nums">5 Days</span>
                 <span className="text-label-md text-on-surface-variant bg-surface-container px-1.5 py-0.5 rounded-md">Best: 12</span>
             </div>
           </div>
        </div>

        <div className="bg-surface-container-lowest p-5 rounded-[24px] border border-outline-variant shadow-sm flex items-center gap-4">
           <div className="w-12 h-12 rounded-full bg-[#fce7f3] dark:bg-pink-950 flex items-center justify-center text-[#be185d] dark:text-pink-300">
             <TrendingUp size={24} />
           </div>
           <div>
             <p className="text-body-md text-on-surface-variant">Weight Trend</p>
             <div className="flex items-center gap-2">
                <span className="text-headline-sm font-bold text-on-surface tabular-nums">-0.5 kg</span>
                <span className="text-xs text-green-600 bg-green-100 px-1.5 py-0.5 rounded-md flex items-center">
                    <ArrowDownRight size={12} className="mr-0.5" /> 
                </span>
             </div>
           </div>
        </div>
      </div>

      {/* Main Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-8">
        
        {/* Weekly Calories Chart */}
        <div className="lg:col-span-2 bg-surface-container-lowest rounded-[32px] p-6 shadow-sm border border-outline-variant">
          <div className="flex justify-between items-center mb-6">
            <h3 className="text-title-lg text-on-surface">Weekly Intake</h3>
            <div className="flex gap-2">
               <span className="text-xs font-medium px-2 py-1 bg-surface-container rounded-full text-on-surface-variant">Calories</span>
            </div>
          </div>
          
          <div className="h-[300px] w-full">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={weeklyData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="rgb(var(--md-outline-variant))" />
                <XAxis 
                    dataKey="day" 
                    axisLine={false} 
                    tickLine={false} 
                    tick={{fill: 'rgb(var(--md-on-surface-variant))', fontSize: 12}} 
                    dy={10}
                />
                <YAxis 
                    axisLine={false} 
                    tickLine={false} 
                    tick={{fill: 'rgb(var(--md-on-surface-variant))', fontSize: 12}} 
                />
                <Tooltip 
                    cursor={{fill: 'rgb(var(--md-surface-container))'}}
                    contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)', backgroundColor: 'rgb(var(--md-surface-container-lowest))', color: 'rgb(var(--md-on-surface))' }}
                />
                <Bar dataKey="calories" radius={[8, 8, 8, 8]} barSize={32}>
                  {weeklyData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.calories > goals.calories ? '#f87171' : 'rgb(var(--md-primary))'} />
                  ))}
                </Bar>
                {/* Reference Line for Goal could be added here */}
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Macro Distribution Pie Chart */}
        <div className="bg-surface-container-lowest rounded-[32px] p-6 shadow-sm border border-outline-variant flex flex-col">
           <h3 className="text-title-lg text-on-surface mb-6">Macro Split</h3>
           <div className="flex-1 min-h-[250px] relative">
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
                    <Tooltip contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)', backgroundColor: 'rgb(var(--md-surface-container-lowest))', color: 'rgb(var(--md-on-surface))' }} />
                    <Legend verticalAlign="bottom" height={36} iconType="circle" />
                </PieChart>
              </ResponsiveContainer>
              {/* Center Text */}
              <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none pb-8">
                  <span className="text-headline-md font-bold text-on-surface">Avg</span>
                  <span className="text-label-md text-on-surface-variant">Distribution</span>
              </div>
           </div>
        </div>
      </div>

      {/* Top Foods / Insights List */}
      <div className="bg-surface-container rounded-[32px] p-6 border border-outline-variant">
          <h3 className="text-title-lg text-on-surface mb-4">Highlights</h3>
          <div className="space-y-3">
             <div className="flex items-center justify-between p-4 bg-surface-container-lowest rounded-2xl">
                <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-rose-100 text-rose-600 flex items-center justify-center">
                        <Flame size={20} />
                    </div>
                    <div>
                        <p className="font-medium text-on-surface">Highest Calorie Day</p>
                        <p className="text-label-md text-on-surface-variant">Saturday</p>
                    </div>
                </div>
                <span className="font-bold text-on-surface tabular-nums">2,600 kcal</span>
             </div>

             <div className="flex items-center justify-between p-4 bg-surface-container-lowest rounded-2xl">
                <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-amber-100 text-amber-600 flex items-center justify-center">
                        <Zap size={20} />
                    </div>
                    <div>
                        <p className="font-medium text-on-surface">Top Nutrient Source</p>
                        <p className="text-label-md text-on-surface-variant">Grilled Chicken Salad</p>
                    </div>
                </div>
                <span className="font-bold text-on-surface tabular-nums">45g Protein</span>
             </div>
          </div>
      </div>
    </div>
  );
};

export default StatsView;
