import { useEffect, useState } from "react";
import { apiGet, apiPost } from "./api/client"; 
import type { Health, Nutrient } from "./api/types";

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [nutrients, setNutrients] = useState<Nutrient[]>([]);
  const [error, setError] = useState<string | null>(null);

  // Form input and login state
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [token, setToken] = useState<string | null>(null);

  useEffect(() => {
    apiGet<Health>("/healthz").then(setHealth).catch((e) => setError(String(e)));
    apiGet<Nutrient[]>("/api/v1/nutrients").then(setNutrients).catch((e) => setError(String(e)));
  }, []);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      // POST data to Vite proxy -> Go backend
      const data = await apiPost<{ token: string }>("/api/v1/auth/login", { email, password });
      setToken(data.token); // Store the returned JWT token
      alert("Logged in successfully!");
    } catch (err) {
      setError(String(err));
    }
  };

  return (
    <main style={{ fontFamily: "system-ui, sans-serif", maxWidth: 720, margin: "40px auto", padding: "0 16px" }}>
      <h1>Food / Nutrition / Food-Loss</h1>
      
      {/* 🔐 NEW LOGIN INTERFACE */}
      <section style={{ marginTop: 24, padding: 16, backgroundColor: "#f5f5f5", borderRadius: 8 }}>
        <h2>Login Form</h2>
        {token ? (
          <div>
            <p style={{ color: "green" }}>🔒 Status: <strong>Logged In</strong></p>
            <button onClick={() => setToken(null)}>Log Out</button>
          </div>
        ) : (
          <form onSubmit={handleLogin} style={{ display: "flex", flexDirection: "column", gap: 8, maxWidth: 300 }}>
            <input 
              type="email" 
              placeholder="Email (test@example.com)" 
              value={email} 
              onChange={(e) => setEmail(e.target.value)} 
              required 
            />
            <input 
              type="password" 
              placeholder="Password (password123)" 
              value={password} 
              onChange={(e) => setPassword(e.target.value)} 
              required 
            />
            <button type="submit">Submit Login</button>
          </form>
        )}
      </section>

      <section style={{ marginTop: 24 }}>
        <h2>Backend health</h2>
        {error && <p style={{ color: "crimson" }}>{error}</p>}
        {health ? (
          <p>status: <strong>{health.status}</strong> · db: <strong>{health.db ? "connected" : "down"}</strong></p>
        ) : (
          !error && <p>checking…</p>
        )}
      </section>
    </main>
  );
}