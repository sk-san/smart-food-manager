import { useState, useEffect, useMemo } from "react";
import { Carrot, LayoutGrid, Leaf, Plus, TrendingUp, User, LogOut } from "lucide-react";
import AddEntryModal from "./components/AddEntryModal";
import CompanionCharacter from "./components/CompanionCharacter";
import DashboardView from "./components/DashboardView";
import LoginView from "./components/LoginView";
import PantryView from "./components/PantryView";
import StatsView from "./components/StatsView";
import SettingsView from "./components/SettingsView";
import {
  FoodEntry,
  DailyGoal,
  GuestRole,
  SUGGESTED_GOALS,
  DEFAULT_PROFILE,
  GUEST_PROFILES,
} from "./types/nutrition";
import { logNavigation, logScreenView } from "./telemetry/events";
import { getToken, setToken } from "./api/client";

type Tab = "dashboard" | "history" | "pantry" | "settings";

const INITIAL_ENTRIES: FoodEntry[] = [];

const TAB_LABELS: Record<Tab, string> = {
  dashboard: "Today",
  history: "Stats",
  pantry: "Pantry",
  settings: "Account",
};

function App() {
  const [entries, setEntries] = useState<FoodEntry[]>(INITIAL_ENTRIES);
  const [goals, setGoals] = useState<DailyGoal>(SUGGESTED_GOALS);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<Tab>("dashboard");
  const [isLookingAtContent, setIsLookingAtContent] = useState(false);
  
  const [isLoggedIn, setIsLoggedIn] = useState(() => !!getToken());
  
  const [guestRole, setGuestRole] = useState<GuestRole | null>(null);

  const profile = guestRole ? GUEST_PROFILES[guestRole] : DEFAULT_PROFILE;
  const [isDark, setIsDark] = useState(() => document.documentElement.classList.contains("dark"));

  const handleLogout = () => {
    logNavigation(activeTab, "login"); 
    setToken(null);
    setIsLoggedIn(false);
    setGuestRole(null);
    logScreenView("login"); 
  };

  const handleToggleDark = () => {
    setIsDark((prev) => {
      const next = !prev;
      document.documentElement.classList.toggle("dark", next);
      localStorage.setItem("nutrimind-theme", next ? "dark" : "light");
      return next;
    });
  };

  useEffect(() => {
    logScreenView("dashboard");
  }, []);

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

  const enterApp = (from: string) => {
    setIsLoggedIn(true);
    setActiveTab("dashboard");
    logNavigation(from, "dashboard");
    logScreenView("dashboard");
  };

  const handleSignIn = () => enterApp("login");

  const handleGuestLogin = (role: GuestRole) => {
    setGuestRole(role);
    enterApp(`login:guest-${role}`);
  };

  const navItems: { tab: Tab; icon: typeof LayoutGrid }[] = [
    { tab: "dashboard", icon: LayoutGrid },
    { tab: "history", icon: TrendingUp },
    { tab: "pantry", icon: Carrot },
    { tab: "settings", icon: User },
  ];

  if (!isLoggedIn) {
    return <LoginView onSignIn={handleSignIn} onGuestLogin={handleGuestLogin} onLoggedIn={() => setIsLoggedIn(true)} />;
  }

  const brand = (
    <button
      onClick={() => handleTabChange("dashboard")}
      aria-label="Today"
      className="flex items-center gap-2.5"
    >
      <div className="grid h-9 w-9 place-items-center rounded-full bg-accent-2-300 transition-transform hover:scale-105 active:scale-95">
        <Leaf size={18} strokeWidth={2.75} className="text-accent-2-800" />
      </div>
      <span className="font-display text-[19px] text-ink">Nutri</span>
    </button>
  );

  const avatar = (
    <button
      onClick={() => handleTabChange("settings")}
      aria-label="Account"
      aria-current={activeTab === "settings" ? "page" : undefined}
      className="grid h-9 w-9 place-items-center rounded-full bg-accent-300 text-[13px] font-semibold text-accent-900 transition-transform hover:scale-105 active:scale-95"
    >
      {profile.initials}
    </button>
  );

  return (
    <div className="flex min-h-screen flex-col bg-bg">
      {/* Top navigation (desktop) */}
      <header className="mx-auto hidden w-full max-w-6xl items-center gap-7 px-8 pt-7 md:flex">
        <div className="mr-auto">{brand}</div>
        <nav className="flex items-center gap-6">
          {navItems.map(({ tab }) => (
            <button
              key={tab}
              onClick={() => handleTabChange(tab)}
              aria-current={activeTab === tab ? "page" : undefined}
              className={`text-[13px] transition-colors ${
                activeTab === tab
                  ? "font-semibold text-accent-700"
                  : "text-neutral-600 hover:text-ink"
              }`}
            >
              {TAB_LABELS[tab]}
            </button>
          ))}
        </nav>
        <button onClick={() => setIsModalOpen(true)} className="btn btn-primary shadow-sm">
          <Plus size={15} strokeWidth={2.75} />
          Log food
        </button>
        {avatar}
        <button
          onClick={handleLogout}
          className="ml-2 flex items-center gap-1 px-3 py-1.5 rounded-lg text-neutral-600 hover:text-red-600 hover:bg-neutral-100 transition-all text-[13px] font-medium"
        >
          <LogOut size={16} />
          <span>Log Out</span>
        </button>
      </header>

      {/* Compact top bar (mobile) */}
      <div className="flex items-center justify-between px-5 pt-5 md:hidden">
        {brand}
        <div className="flex items-center gap-3">
          {avatar}
          <button onClick={handleLogout} className="text-neutral-500 hover:text-red-600">
            <LogOut size={20} />
          </button>
        </div>
      </div>

      {/* Main Content */}
      <main className="relative mx-auto w-full max-w-6xl flex-1 px-5 pb-32 pt-5 md:px-8 md:pb-14 md:pt-6">
        {activeTab === "dashboard" ? (
          <DashboardView
            todayTotals={todayTotals}
            goals={goals}
            entries={entries}
            onLogFood={() => setIsModalOpen(true)}
            onOpenStats={() => handleTabChange("history")}
            onHoverStart={handleContentHoverStart}
            onHoverEnd={handleContentHoverEnd}
          />
        ) : activeTab === "history" ? (
          <StatsView entries={entries} goals={goals} />
        ) : activeTab === "pantry" ? (
          <PantryView onHoverStart={handleContentHoverStart} onHoverEnd={handleContentHoverEnd} />
        ) : (
          <SettingsView
            goals={goals}
            onUpdateGoals={setGoals}
            isDark={isDark}
            onToggleDark={handleToggleDark}
            onSignOut={handleLogout}
            profile={profile}
          />
        )}

        {/* Cute AI Character - Persistent across views */}
        <CompanionCharacter stats={todayTotals} goals={goals} isLookingAtScreen={isLookingAtContent} />
      </main>

      {/* Floating Action Button (mobile) */}
      <button
        onClick={() => setIsModalOpen(true)}
        className="fixed bottom-24 right-5 z-50 grid h-14 w-14 place-items-center rounded-full bg-accent text-bg shadow-md transition-transform hover:scale-105 active:scale-95 md:hidden"
        aria-label="Log food"
      >
        <Plus size={26} strokeWidth={2.75} />
      </button>

      <AddEntryModal isOpen={isModalOpen} onClose={() => setIsModalOpen(false)} onAdd={handleAddEntries} />

      {/* Mobile Bottom Nav */}
      <div className="fixed inset-x-0 bottom-0 z-40 flex h-20 items-center justify-around border-t border-divider bg-surface px-4 md:hidden">
        {navItems.map(({ tab, icon: Icon }) => (
          <button
            key={tab}
            onClick={() => handleTabChange(tab)}
            className={`flex flex-col items-center gap-1 p-2 transition-colors ${
              activeTab === tab ? "text-accent-700" : "text-neutral-500"
            }`}
          >
            <Icon size={23} strokeWidth={2.75} />
            <span className="text-[11px] font-semibold">{TAB_LABELS[tab]}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

export default App;