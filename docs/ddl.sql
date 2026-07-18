-- =============================================================================
--  Smart Food Manager — DDL (original to this repo)
-- =============================================================================
--  This schema is derived from what the code in this repository actually does,
--  rather than from the product spec:
--
--    * backend/internal/handler/nutrients.go  — reads the nutrients master
--    * backend/internal/handler/labels.go     — inserts foods + food_nutrients
--                                               (label-image extraction flow)
--    * backend/internal/handler/auth.go       — JWT login (subject = email,
--                                               roles: 'user'; 'admin' gates
--                                               the telemetry ingest route)
--    * frontend/src/types/nutrition.ts        — FoodEntry / DailyGoal, the
--                                               dashboard's core domain model
--    * frontend/src/App.tsx                   — DEFAULT_GOALS reference values
--
--  Relationship to backend/migrations/0001_init.sql:
--    0001 is the spec-driven schema (inventory, meals, waste events, roll-up
--    views) and is what docker-compose applies on first DB init. This file is
--    the as-built core: the tables the code uses today, plus persistence for
--    the frontend's FoodEntry / DailyGoal model, which currently lives only in
--    React state. Table/column names shared with 0001 are kept identical so
--    the backend queries work unchanged against either schema.
--
--  Intended use: run against a FRESH database (not idempotent on top of 0001).
--
--  Conventions (same as 0001): UUID PKs, timestamptz, NUMERIC for amounts.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "citext";     -- case-insensitive email

-- -----------------------------------------------------------------------------
--  ENUM TYPES — only the vocabulary the code emits
-- -----------------------------------------------------------------------------

-- normalizeFoodType() in labels.go emits exactly these two values
CREATE TYPE food_type AS ENUM ('raw_material', 'prepared_food');

-- nutrients.focus, read by GET /api/v1/nutrients
CREATE TYPE nutrient_focus AS ENUM (
    'deficiency_watch',   -- protein, calcium, iron ...
    'excess_watch',       -- calories, sodium, fat ...
    'caution'
);

-- =============================================================================
--  USERS & RBAC  (backs the JWT/RBAC middleware; login is stubbed today and
--  auth.go says "Replace with a real lookup against the users table")
-- =============================================================================

CREATE TABLE roles (
    id          SMALLSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT
);

-- the two roles the code actually checks: every login gets 'user',
-- RequireRole("admin") protects the telemetry ingest route (server.go)
INSERT INTO roles (name, description) VALUES
    ('user',  'Default role attached to every login'),
    ('admin', 'Required for privileged routes (telemetry ingest)');

CREATE TABLE users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email          CITEXT NOT NULL UNIQUE,     -- JWT subject is the email
    password_hash  TEXT,
    display_name   TEXT,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id    SMALLINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

-- =============================================================================
--  NUTRIENTS MASTER  (columns exactly as queried by nutrients.go / labels.go)
-- =============================================================================

CREATE TABLE nutrients (
    id                     SMALLSERIAL PRIMARY KEY,
    code                   TEXT NOT NULL UNIQUE,
    name                   TEXT NOT NULL,
    unit                   TEXT NOT NULL,           -- 'kcal', 'g', 'mg'
    focus                  nutrient_focus NOT NULL,
    reference_daily_amount NUMERIC(10,3),
    sort_order             SMALLINT NOT NULL DEFAULT 100,
    is_active              BOOLEAN NOT NULL DEFAULT TRUE
);

-- Seed: the seven nutrients the frontend tracks (NutritionData in
-- frontend/src/types/nutrition.ts). Codes reuse the frontend field names so
-- the label-extraction prompt and the UI speak the same vocabulary.
-- reference_daily_amount mirrors DEFAULT_GOALS in frontend/src/App.tsx.
INSERT INTO nutrients (code, name, unit, focus, reference_daily_amount, sort_order) VALUES
    ('calories', 'Energy',       'kcal', 'excess_watch',     2200, 10),
    ('protein',  'Protein',      'g',    'deficiency_watch',  150, 20),
    ('carbs',    'Carbohydrate', 'g',    'excess_watch',      250, 30),
    ('fat',      'Fat',          'g',    'excess_watch',       70, 40),
    ('sodium',   'Sodium',       'mg',   'excess_watch',     2300, 50),
    ('calcium',  'Calcium',      'mg',   'deficiency_watch', 1000, 60),
    ('iron',     'Iron',         'mg',   'deficiency_watch',   18, 70);

-- =============================================================================
--  FOODS CATALOG  (written by the label-extraction flow in labels.go)
-- =============================================================================

CREATE TABLE foods (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    food_type  food_type NOT NULL,
    category   TEXT,                              -- nullString() may pass NULL
    is_global  BOOLEAN NOT NULL DEFAULT TRUE,     -- labels.go inserts false
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_foods_type ON foods(food_type);
CREATE INDEX idx_foods_name ON foods(lower(name));

-- per-100 g nutrient content; the composite PK is required by the
-- ON CONFLICT (food_id, nutrient_id) upsert in labels.go
CREATE TABLE food_nutrients (
    food_id         UUID NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    nutrient_id     SMALLINT NOT NULL REFERENCES nutrients(id) ON DELETE CASCADE,
    amount_per_100g NUMERIC(12,4) NOT NULL CHECK (amount_per_100g >= 0),
    PRIMARY KEY (food_id, nutrient_id)
);

-- =============================================================================
--  FOOD ENTRIES  (persists frontend FoodEntry, today React-state only)
-- =============================================================================
--  Fixed nutrient columns (not an entry_nutrients join table) on purpose:
--  FoodEntry / AnalyzedFoodItem are a fixed seven-field shape end to end
--  (Gemini analysis -> API -> UI), so fixed columns keep reads a single-row
--  fetch and match the repo's own types. Foods added via label extraction can
--  additionally link to the catalog through food_id.

CREATE TABLE food_entries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    food_id     UUID REFERENCES foods(id) ON DELETE SET NULL,  -- optional catalog link

    name        TEXT NOT NULL,
    icon        TEXT,                            -- emoji shown on the dashboard
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- NutritionData, per entry (absolute amounts, not per-100 g)
    calories    NUMERIC(7,1) NOT NULL DEFAULT 0 CHECK (calories >= 0),  -- kcal
    protein     NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (protein  >= 0),  -- g
    carbs       NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (carbs    >= 0),  -- g
    fat         NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (fat      >= 0),  -- g
    sodium      NUMERIC(8,2) NOT NULL DEFAULT 0 CHECK (sodium   >= 0),  -- mg
    calcium     NUMERIC(8,2) NOT NULL DEFAULT 0 CHECK (calcium  >= 0),  -- mg
    iron        NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (iron     >= 0),  -- mg

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_food_entries_user_time ON food_entries(user_id, consumed_at DESC);

-- =============================================================================
--  DAILY GOALS  (persists frontend DailyGoal; defaults = DEFAULT_GOALS)
-- =============================================================================

CREATE TABLE daily_goals (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    calories   NUMERIC(7,1) NOT NULL DEFAULT 2200 CHECK (calories > 0),
    protein    NUMERIC(7,2) NOT NULL DEFAULT 150  CHECK (protein  > 0),
    carbs      NUMERIC(7,2) NOT NULL DEFAULT 250  CHECK (carbs    > 0),
    fat        NUMERIC(7,2) NOT NULL DEFAULT 70   CHECK (fat      > 0),
    sodium     NUMERIC(8,2) NOT NULL DEFAULT 2300 CHECK (sodium   > 0),
    calcium    NUMERIC(8,2) NOT NULL DEFAULT 1000 CHECK (calcium  > 0),
    iron       NUMERIC(7,2) NOT NULL DEFAULT 18   CHECK (iron     > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
--  ROLL-UP VIEW  — feeds the dashboard rings and the Stats tab
-- =============================================================================

CREATE VIEW v_entry_daily_totals AS
SELECT user_id,
       date_trunc('day', consumed_at)::date AS day,
       COUNT(*)      AS entry_count,
       SUM(calories) AS calories,
       SUM(protein)  AS protein,
       SUM(carbs)    AS carbs,
       SUM(fat)      AS fat,
       SUM(sodium)   AS sodium,
       SUM(calcium)  AS calcium,
       SUM(iron)     AS iron
FROM food_entries
GROUP BY user_id, day;
