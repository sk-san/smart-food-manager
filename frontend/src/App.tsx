import { useState, useEffect, useMemo } from "react";
import { LayoutDashboard, Utensils, Settings, Plus, TrendingUp } from "lucide-react";
import AddEntryModal from "./components/AddEntryModal";
import CompanionCharacter from "./components/CompanionCharacter";
import DashboardView from "./components/DashboardView";
import StatsView from "./components/StatsView";
import SettingsView from "./components/SettingsView";
import { FoodEntry, DailyGoal } from "./types/nutrition";
import { logNavigation, logScreenView } from "./telemetry/events";

type Tab = "dashboard" | "history" | "settings";

// Demo seed data. Entries live in local state for now; persistence through
// the backend is a follow-up — the AI analysis and companion already route
// through the instrumented API client.
const INITIAL_ENTRIES: FoodEntry[] = [
  { id: "1", name: "Oatmeal & Blueberries", calories: 350, protein: 12, carbs: 60, fat: 6, sodium: 150, calcium: 80, iron: 3.5, timestamp: Date.now() - 10000000 },
  { id: "2", name: "Grilled Chicken Salad", calories: 450, protein: 45, carbs: 12, fat: 20, sodium: 600, calcium: 120, iron: 2.1, timestamp: Date.now() - 5000000 },
];

const DEFAULT_GOALS: DailyGoal = {
  calories: 2200,
  protein: 150,
  carbs: 250,
  fat: 70,
  sodium: 2300,
  calcium: 1000,
  iron: 18,
};

const TAB_LABELS: Record<Tab, string> = {
  dashboard: "Dash",
  history: "Stats",
  settings: "Prefs",
};

function App() {
  const [entries, setEntries] = useState<FoodEntry[]>(INITIAL_ENTRIES);
  const [goals, setGoals] = useState<DailyGoal>(DEFAULT_GOALS);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<Tab>("dashboard");
  const [isLookingAtContent, setIsLookingAtContent] = useState(false);

  // Initial screen view for telemetry.
  useEffect(() => {
    logScreenView("dashboard");
  }, []);

  // Switches tabs and emits a navigation event so the frontend signal lines
  // up with backend traces in Grafana.
  const handleTabChange = (next: Tab) => {
    if (next === activeTab) return;
    logNavigation(activeTab, next);
    setActiveTab(next);
    logScreenView(next);
  };

  const todayTotals = useMemo(() => {
    return entries.reduce(
      (acc, curr) => ({
        calories: acc.calories + curr.calories,
        protein: acc.protein + curr.protein,
        carbs: acc.carbs + curr.carbs,
        fat: acc.fat + curr.fat,
        sodium: acc.sodium + curr.sodium,
        calcium: acc.calcium + curr.calcium,
        iron: acc.iron + curr.iron,
      }),
      { calories: 0, protein: 0, carbs: 0, fat: 0, sodium: 0, calcium: 0, iron: 0 }
    );
  }, [entries]);

  const handleAddEntries = (newItems: Omit<FoodEntry, "id" | "timestamp">[]) => {
    const itemsWithMeta = newItems.map((item) => ({
      ...item,
      id: crypto.randomUUID(),
      timestamp: Date.now(),
    }));
    setEntries((prev) => [...prev, ...itemsWithMeta]);
  };

  const handleContentHoverStart = () => setIsLookingAtContent(true);
  const handleContentHoverEnd = () => setIsLookingAtContent(false);

  const navItems: { tab: Tab; icon: typeof LayoutDashboard }[] = [
    { tab: "dashboard", icon: LayoutDashboard },
    { tab: "history", icon: TrendingUp },
    { tab: "settings", icon: Settings },
  ];

  return (
    <div className="min-h-screen bg-[#fdfcff] flex">
      {/* Navigation Rail (Desktop) */}
      <aside className="hidden md:flex flex-col w-24 bg-[#f3f0f5] border-r border-[#e7e0ec] items-center py-8 gap-8 fixed h-full z-20">
        <div className="w-12 h-12 bg-[#eaddff] rounded-xl flex items-center justify-center mb-4">
          <Utensils className="text-[#21005d]" size={24} />
        </div>

        <nav className="flex flex-col gap-6 w-full items-center">
          {navItems.map(({ tab, icon: Icon }) => (
            <button
              key={tab}
              onClick={() => handleTabChange(tab)}
              className={`flex flex-col items-center gap-1 w-16 py-2 rounded-xl transition-all ${
                activeTab === tab ? "bg-[#e8def8] text-[#1d1b20]" : "text-[#49454f] hover:bg-[#e7e0ec]/50"
              }`}
            >
              <Icon size={24} strokeWidth={activeTab === tab ? 2.5 : 2} />
              <span className="text-[10px] font-medium">{TAB_LABELS[tab]}</span>
            </button>
          ))}
        </nav>
      </aside>

      {/* Main Content */}
      <main className="flex-1 md:ml-24 pb-24 md:pb-8 p-4 md:p-8 max-w-7xl mx-auto w-full relative">
        {activeTab === "dashboard" ? (
          <DashboardView
            todayTotals={todayTotals}
            goals={goals}
            entries={entries}
            onHoverStart={handleContentHoverStart}
            onHoverEnd={handleContentHoverEnd}
          />
        ) : activeTab === "history" ? (
          <StatsView entries={entries} goals={goals} />
        ) : (
          <SettingsView goals={goals} onUpdateGoals={setGoals} />
        )}

        {/* Cute AI Character - Persistent across views */}
        <CompanionCharacter stats={todayTotals} goals={goals} isLookingAtScreen={isLookingAtContent} />
      </main>

      {/* Floating Action Button (FAB) */}
      <button
        onClick={() => setIsModalOpen(true)}
        className="fixed bottom-6 right-6 md:bottom-10 md:right-10 w-16 h-16 bg-[#6750a4] hover:bg-[#6750a4]/90 text-white rounded-[20px] shadow-xl shadow-[#6750a4]/30 flex items-center justify-center transition-all hover:scale-105 active:scale-95 z-50 group"
        aria-label="Log food"
      >
        <Plus size={32} className="group-hover:rotate-90 transition-transform duration-300" />
      </button>

      <AddEntryModal isOpen={isModalOpen} onClose={() => setIsModalOpen(false)} onAdd={handleAddEntries} />

      {/* Mobile Bottom Nav */}
      <div className="md:hidden fixed bottom-0 left-0 right-0 h-20 bg-[#f3f0f5] border-t border-[#e7e0ec] flex justify-around items-center px-4 z-40">
        {navItems.map(({ tab, icon: Icon }) => (
          <button
            key={tab}
            onClick={() => handleTabChange(tab)}
            className={`flex flex-col items-center gap-1 p-2 rounded-full transition-all ${
              activeTab === tab ? "text-[#1d1b20]" : "text-[#49454f]"
            }`}
          >
            <div className={`px-5 py-1 rounded-full ${activeTab === tab ? "bg-[#e8def8]" : ""}`}>
              <Icon size={24} strokeWidth={activeTab === tab ? 2.5 : 2} />
            </div>
            <span className="text-xs font-medium">{TAB_LABELS[tab]}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

export default App;
