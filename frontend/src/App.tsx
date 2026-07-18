import { useState, useEffect, useMemo } from "react";
import { LayoutGrid, Leaf, Plus, TrendingUp, User } from "lucide-react";
import AddEntryModal from "./components/AddEntryModal";
import CompanionCharacter from "./components/CompanionCharacter";
import DashboardView from "./components/DashboardView";
import LoginView from "./components/LoginView";
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

type Tab = "dashboard" | "history" | "settings";

// Demo seed data. Entries live in local state for now; persistence through
// the backend is a follow-up — the AI analysis and companion already route
// through the instrumented API client.
const INITIAL_ENTRIES: FoodEntry[] = [
 ];

const TAB_LABELS: Record<Tab, string> = {
  dashboard: "Today",
  history: "Stats",
  settings: "Account",
};

function App() {
  const [entries, setEntries] = useState<FoodEntry[]>(INITIAL_ENTRIES);
  const [goals, setGoals] = useState<DailyGoal>(SUGGESTED_GOALS);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<Tab>("dashboard");
  const [isLookingAtContent, setIsLookingAtContent] = useState(false);
  // Template-only sign-in screen (design 2c): "Sign out" shows it, "Sign in"
  // returns. No real auth behind it yet — the backend JWT flow is a follow-up.
  const [showLogin, setShowLogin] = useState(false);
  // Set by the login screen's guest bypass; null means the demo account.
  const [guestRole, setGuestRole] = useState<GuestRole | null>(null);

  const profile = guestRole ? GUEST_PROFILES[guestRole] : DEFAULT_PROFILE;
  // Initial value comes from the pre-paint script in index.html, which
  // resolves localStorage + the system preference before React mounts.
  const [isDark, setIsDark] = useState(() => document.documentElement.classList.contains("dark"));

  const handleToggleDark = () => {
    setIsDark((prev) => {
      const next = !prev;
      document.documentElement.classList.toggle("dark", next);
      localStorage.setItem("nutrimind-theme", next ? "dark" : "light");
      return next;
    });
  };

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

  const handleSignOut = () => {
    logNavigation(activeTab, "login");
    setShowLogin(true);
    setGuestRole(null);
    logScreenView("login");
  };

  const enterApp = (from: string) => {
    setShowLogin(false);
    setActiveTab("dashboard");
    logNavigation(from, "dashboard");
    logScreenView("dashboard");
  };

  const handleSignIn = () => enterApp("login");

  // Auth bypass for testing: no credentials, just a presentational role.
  const handleGuestLogin = (role: GuestRole) => {
    setGuestRole(role);
    enterApp(`login:guest-${role}`);
  };

  const navItems: { tab: Tab; icon: typeof LayoutGrid }[] = [
    { tab: "dashboard", icon: LayoutGrid },
    { tab: "history", icon: TrendingUp },
    { tab: "settings", icon: User },
  ];

  if (showLogin) {
    return <LoginView onSignIn={handleSignIn} onGuestLogin={handleGuestLogin} />;
  }

  const brand = (
    <div className="flex items-center gap-2.5">
      <div className="grid h-9 w-9 place-items-center rounded-full bg-accent-2-300">
        <Leaf size={18} strokeWidth={2.75} className="text-accent-2-800" />
      </div>
      <span className="font-display text-[19px] text-ink">Nutri</span>
    </div>
  );

  const avatar = (
    <div className="grid h-9 w-9 place-items-center rounded-full bg-accent-300 text-[13px] font-semibold text-accent-900">
      {profile.initials}
    </div>
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
      </header>

      {/* Compact top bar (mobile) */}
      <div className="flex items-center justify-between px-5 pt-5 md:hidden">
        {brand}
        {avatar}
      </div>

      {/* Main Content */}
      <main className="relative mx-auto w-full max-w-6xl flex-1 px-5 pb-32 pt-5 md:px-8 md:pb-14 md:pt-6">
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
          <SettingsView
            goals={goals}
            onUpdateGoals={setGoals}
            isDark={isDark}
            onToggleDark={handleToggleDark}
            onSignOut={handleSignOut}
            profile={profile}
          />
        )}

        {/* Cute AI Character - Persistent across views */}
        <CompanionCharacter stats={todayTotals} goals={goals} isLookingAtScreen={isLookingAtContent} />
      </main>

      {/* Floating Action Button (mobile — desktop logs from the header) */}
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
