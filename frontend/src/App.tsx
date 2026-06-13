import { useEffect, useState } from "react";
import { apiGet } from "./api/client";
import type { Health, Nutrient } from "./api/types";
import { logScreenRendered, logScreenView } from "./telemetry/events";

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [nutrients, setNutrients] = useState<Nutrient[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    logScreenView("home");
    logScreenRendered("home", performance.now());
  }, []);

  useEffect(() => {
    apiGet<Health>("/healthz").then(setHealth).catch((e) => setError(String(e)));
    apiGet<Nutrient[]>("/api/v1/nutrients")
      .then(setNutrients)
      .catch((e) => setError(String(e)));
  }, []);

  return (
    <main style={{ fontFamily: "system-ui, sans-serif", maxWidth: 720, margin: "40px auto", padding: "0 16px" }}>
      <h1>Food / Nutrition / Food-Loss</h1>
      <p style={{ color: "#666" }}>Monorepo scaffold — Go backend + React/TS frontend.</p>

      <section style={{ marginTop: 24 }}>
        <h2>Backend health</h2>
        {error && <p style={{ color: "crimson" }}>{error}</p>}
        {health ? (
          <p>
            status: <strong>{health.status}</strong> · db:{" "}
            <strong>{health.db ? "connected" : "down"}</strong>
          </p>
        ) : (
          !error && <p>checking…</p>
        )}
      </section>

      <section style={{ marginTop: 24 }}>
        <h2>Nutrients ({nutrients.length})</h2>
        {nutrients.length === 0 ? (
          <p style={{ color: "#666" }}>
            None yet — run the migration and seed the <code>nutrients</code> table.
          </p>
        ) : (
          <ul>
            {nutrients.map((n) => (
              <li key={n.id}>
                {n.name} ({n.unit}) — {n.focus}
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
