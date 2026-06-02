-- =============================================================================
--  Food / Nutrition / Food-Loss app — PostgreSQL schema
-- =============================================================================
--  Scope (per request):
--    * comprehensive nutrient master + per-food nutrient content
--    * foods catalog covering BOTH raw materials (ingredients) and prepared food
--    * inventory items with a "wasted" flag and consumed / wasted percentages
--    * consumed-nutrient & food-waste roll-ups for day / week / month / year
--    * user accounts + RBAC (matches the JWT / RBAC stack in the spec)
--
--  Conventions:
--    * UUID primary keys, timestamptz everywhere, NUMERIC for amounts.
--    * Quantities are normalized so nutrient math works: food nutrient content
--      is stored per 100 g, meal/inventory quantities are stored in grams.
--    * Percentages are STORED generated columns (no division-by-zero).
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "citext";      -- case-insensitive email

-- -----------------------------------------------------------------------------
--  ENUM TYPES  (vocabulary lifted from the product spec, §10–§11)
-- -----------------------------------------------------------------------------

-- raw material (ingredient) vs. a prepared dish/meal-ready food
CREATE TYPE food_type AS ENUM ('raw_material', 'prepared_food');

-- how the product treats each nutrient (spec §10)
CREATE TYPE nutrient_focus AS ENUM (
    'deficiency_watch',   -- protein, fibre, calcium, iron, potassium ...
    'excess_watch',       -- energy, sodium/salt, saturated fat, free sugar
    'caution'             -- vitamin D, folate, iodine ...
);

CREATE TYPE meal_type AS ENUM ('breakfast', 'lunch', 'dinner', 'snack', 'other');

CREATE TYPE storage_location AS ENUM ('fridge', 'freezer', 'pantry', 'other');

-- structured food-loss logging (spec §11)
CREATE TYPE date_label_type AS ENUM (
    'best_before', 'use_by', 'display_until', 'no_date_label', 'unknown'
);

CREATE TYPE package_status AS ENUM (
    'unopened', 'opened', 'cooked', 'leftover', 'unknown'
);

CREATE TYPE date_status_at_discard AS ENUM (
    'before_date', 'on_date', '1_3_days_after', '4_7_days_after',
    '8_14_days_after', '15_plus_days_after', 'unknown'
);

CREATE TYPE spoilage_evidence AS ENUM (
    'none', 'visual_mold', 'discoloration', 'smell',
    'texture_change', 'taste', 'unknown'
);

CREATE TYPE discard_reason AS ENUM (
    'expired_best_before', 'expired_use_by', 'near_expiry_but_not_used',
    'spoiled_visible', 'smelled_or_tasted_bad', 'forgot_item_existed',
    'overbought', 'cooked_too_much', 'leftover_not_eaten',
    'unsure_if_safe', 'storage_failure', 'preference_changed', 'other'
);

-- analysis bucket (spec §11 "分析分類")
CREATE TYPE waste_classification AS ENUM (
    'expiry_caused',     -- 期限起因ロス
    'expiry_related',    -- 期限関連ロス
    'expiry_unrelated'   -- 期限非関連ロス
);

-- =============================================================================
--  USERS & RBAC
-- =============================================================================

CREATE TABLE roles (
    id          SMALLSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,          -- e.g. 'admin', 'user'
    description TEXT
);

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           CITEXT NOT NULL UNIQUE,
    password_hash   TEXT,                      -- null when SSO-only
    display_name    TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    email_verified  BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- many-to-many users <-> roles (RBAC)
CREATE TABLE user_roles (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     SMALLINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

-- lightweight profile / preferences (household size, goals, locale)
CREATE TABLE user_profiles (
    user_id          UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    household_size   SMALLINT CHECK (household_size > 0),
    locale           TEXT DEFAULT 'ja-JP',
    timezone         TEXT DEFAULT 'Asia/Tokyo',
    daily_energy_goal_kcal NUMERIC(7,1),       -- optional, for tendency hints
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
--  NUTRIENTS  (comprehensive master)
-- =============================================================================

CREATE TABLE nutrients (
    id                   SMALLSERIAL PRIMARY KEY,
    code                 TEXT NOT NULL UNIQUE,          -- 'protein', 'sodium' ...
    name                 TEXT NOT NULL,
    unit                 TEXT NOT NULL,                 -- 'g', 'mg', 'ug', 'kcal'
    focus                nutrient_focus NOT NULL,
    -- reference intake used only to derive a soft "tendency", not a diagnosis
    reference_daily_amount NUMERIC(10,3),
    sort_order           SMALLINT NOT NULL DEFAULT 100,
    is_active            BOOLEAN NOT NULL DEFAULT TRUE
);

-- =============================================================================
--  FOODS  (raw materials AND prepared food in one catalog)
-- =============================================================================

CREATE TABLE foods (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    food_type       food_type NOT NULL,            -- raw_material | prepared_food
    category        TEXT,                          -- 'vegetable', 'dairy', ...
    -- nutrient content is stored per 100 g of this food (see food_nutrients)
    default_unit    TEXT NOT NULL DEFAULT 'g',
    grams_per_unit  NUMERIC(10,2),                 -- e.g. 1 egg = 50 g; null = by weight
    is_global       BOOLEAN NOT NULL DEFAULT TRUE,  -- shared catalog vs user-created
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_foods_type ON foods(food_type);
CREATE INDEX idx_foods_name ON foods(lower(name));

-- nutrient content per 100 g of a food (the comprehensive nutrient table)
CREATE TABLE food_nutrients (
    food_id         UUID NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    nutrient_id     SMALLINT NOT NULL REFERENCES nutrients(id) ON DELETE CASCADE,
    amount_per_100g NUMERIC(12,4) NOT NULL CHECK (amount_per_100g >= 0),
    PRIMARY KEY (food_id, nutrient_id)
);

-- a prepared_food can optionally be defined as a recipe of raw materials.
-- (purely descriptive; nutrient totals still come from food_nutrients above)
CREATE TABLE recipe_ingredients (
    prepared_food_id UUID NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
    ingredient_id    UUID NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    quantity_g       NUMERIC(10,2) NOT NULL CHECK (quantity_g > 0),
    PRIMARY KEY (prepared_food_id, ingredient_id),
    CHECK (prepared_food_id <> ingredient_id)
);

-- =============================================================================
--  INVENTORY  (a user's stock, with the wasted flag + consumed/wasted %)
-- =============================================================================

CREATE TABLE inventory_items (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    food_id            UUID NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,

    quantity_purchased NUMERIC(10,2) NOT NULL CHECK (quantity_purchased > 0), -- grams
    quantity_consumed  NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (quantity_consumed >= 0),
    quantity_wasted    NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (quantity_wasted >= 0),

    purchase_date      DATE,
    best_before_date   DATE,
    use_by_date        DATE,
    date_label         date_label_type NOT NULL DEFAULT 'unknown',
    storage            storage_location NOT NULL DEFAULT 'other',
    package            package_status NOT NULL DEFAULT 'unopened',

    -- "wasted flag": TRUE once any part of the item has been discarded
    is_wasted          BOOLEAN NOT NULL DEFAULT FALSE,
    -- TRUE once nothing is left to track (consumed + wasted >= purchased)
    is_resolved        BOOLEAN NOT NULL DEFAULT FALSE,

    -- percentages, computed and stored (guarded against /0)
    consumed_pct NUMERIC(5,2) GENERATED ALWAYS AS (
        ROUND(100 * quantity_consumed / NULLIF(quantity_purchased, 0), 2)
    ) STORED,
    wasted_pct   NUMERIC(5,2) GENERATED ALWAYS AS (
        ROUND(100 * quantity_wasted   / NULLIF(quantity_purchased, 0), 2)
    ) STORED,

    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_qty_not_over
        CHECK (quantity_consumed + quantity_wasted <= quantity_purchased)
);

CREATE INDEX idx_inventory_user        ON inventory_items(user_id);
CREATE INDEX idx_inventory_user_open   ON inventory_items(user_id) WHERE is_resolved = FALSE;
CREATE INDEX idx_inventory_expiry      ON inventory_items(user_id, best_before_date, use_by_date);

-- =============================================================================
--  MEALS & CONSUMPTION  (drives the "consumed nutrients" roll-ups)
-- =============================================================================

CREATE TABLE meals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    meal_type    meal_type NOT NULL DEFAULT 'other',
    consumed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    photo_url    TEXT,                              -- spec: photo-based logging
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_meals_user_time ON meals(user_id, consumed_at);

CREATE TABLE meal_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meal_id           UUID NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    food_id           UUID NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
    -- optional link: this portion was drawn from the user's stock
    inventory_item_id UUID REFERENCES inventory_items(id) ON DELETE SET NULL,
    quantity_g        NUMERIC(10,2) NOT NULL CHECK (quantity_g > 0)
);

CREATE INDEX idx_meal_items_meal ON meal_items(meal_id);
CREATE INDEX idx_meal_items_food ON meal_items(food_id);

-- =============================================================================
--  WASTE EVENTS  (the "food wasted" facts, with full §11 logging)
-- =============================================================================

CREATE TABLE waste_events (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    inventory_item_id  UUID REFERENCES inventory_items(id) ON DELETE SET NULL,
    food_id            UUID NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,

    quantity_g         NUMERIC(10,2) NOT NULL CHECK (quantity_g > 0),
    wasted_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    reason             discard_reason NOT NULL,
    date_label         date_label_type NOT NULL DEFAULT 'unknown',
    date_status        date_status_at_discard NOT NULL DEFAULT 'unknown',
    package            package_status NOT NULL DEFAULT 'unknown',
    spoilage           spoilage_evidence NOT NULL DEFAULT 'unknown',
    classification     waste_classification,  -- derived/analytic bucket
    note               TEXT,

    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_waste_user_time ON waste_events(user_id, wasted_at);
CREATE INDEX idx_waste_food      ON waste_events(food_id);

-- =============================================================================
--  ROLL-UP VIEWS  — daily / weekly / monthly / yearly
-- =============================================================================
--  Helper note: change date_trunc('day'|'week'|'month'|'year', ...) to get the
--  bucket you need. Daily views are defined explicitly; the others reuse the
--  same shape. For dashboards at scale, promote these to MATERIALIZED VIEWS.

-- --- Consumed nutrients --------------------------------------------------------
-- grams consumed / 100 * (nutrient per 100 g) = nutrient consumed
CREATE VIEW v_nutrient_intake_daily AS
SELECT m.user_id,
       date_trunc('day', m.consumed_at)::date            AS bucket,
       fn.nutrient_id,
       n.code                                            AS nutrient_code,
       n.unit,
       SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)   AS amount
FROM meals m
JOIN meal_items     mi ON mi.meal_id    = m.id
JOIN food_nutrients fn ON fn.food_id    = mi.food_id
JOIN nutrients      n  ON n.id          = fn.nutrient_id
GROUP BY m.user_id, bucket, fn.nutrient_id, n.code, n.unit;

CREATE VIEW v_nutrient_intake_weekly AS
SELECT m.user_id,
       date_trunc('week', m.consumed_at)::date           AS bucket,
       fn.nutrient_id, n.code AS nutrient_code, n.unit,
       SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)   AS amount
FROM meals m
JOIN meal_items mi     ON mi.meal_id = m.id
JOIN food_nutrients fn ON fn.food_id = mi.food_id
JOIN nutrients n       ON n.id = fn.nutrient_id
GROUP BY m.user_id, bucket, fn.nutrient_id, n.code, n.unit;

CREATE VIEW v_nutrient_intake_monthly AS
SELECT m.user_id,
       date_trunc('month', m.consumed_at)::date          AS bucket,
       fn.nutrient_id, n.code AS nutrient_code, n.unit,
       SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)   AS amount
FROM meals m
JOIN meal_items mi     ON mi.meal_id = m.id
JOIN food_nutrients fn ON fn.food_id = mi.food_id
JOIN nutrients n       ON n.id = fn.nutrient_id
GROUP BY m.user_id, bucket, fn.nutrient_id, n.code, n.unit;

CREATE VIEW v_nutrient_intake_yearly AS
SELECT m.user_id,
       date_trunc('year', m.consumed_at)::date           AS bucket,
       fn.nutrient_id, n.code AS nutrient_code, n.unit,
       SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)   AS amount
FROM meals m
JOIN meal_items mi     ON mi.meal_id = m.id
JOIN food_nutrients fn ON fn.food_id = mi.food_id
JOIN nutrients n       ON n.id = fn.nutrient_id
GROUP BY m.user_id, bucket, fn.nutrient_id, n.code, n.unit;

-- --- Food wasted ---------------------------------------------------------------
CREATE VIEW v_waste_daily AS
SELECT user_id,
       date_trunc('day', wasted_at)::date  AS bucket,
       classification,
       COUNT(*)             AS event_count,
       SUM(quantity_g)      AS grams_wasted
FROM waste_events
GROUP BY user_id, bucket, classification;

CREATE VIEW v_waste_weekly AS
SELECT user_id, date_trunc('week', wasted_at)::date AS bucket, classification,
       COUNT(*) AS event_count, SUM(quantity_g) AS grams_wasted
FROM waste_events GROUP BY user_id, bucket, classification;

CREATE VIEW v_waste_monthly AS
SELECT user_id, date_trunc('month', wasted_at)::date AS bucket, classification,
       COUNT(*) AS event_count, SUM(quantity_g) AS grams_wasted
FROM waste_events GROUP BY user_id, bucket, classification;

CREATE VIEW v_waste_yearly AS
SELECT user_id, date_trunc('year', wasted_at)::date AS bucket, classification,
       COUNT(*) AS event_count, SUM(quantity_g) AS grams_wasted
FROM waste_events GROUP BY user_id, bucket, classification;

-- --- Per-user consumed-vs-wasted summary (overall percentages) -----------------
CREATE VIEW v_user_waste_summary AS
SELECT user_id,
       SUM(quantity_purchased)                                   AS total_purchased_g,
       SUM(quantity_consumed)                                    AS total_consumed_g,
       SUM(quantity_wasted)                                      AS total_wasted_g,
       ROUND(100 * SUM(quantity_consumed)
             / NULLIF(SUM(quantity_purchased), 0), 2)            AS consumed_pct,
       ROUND(100 * SUM(quantity_wasted)
             / NULLIF(SUM(quantity_purchased), 0), 2)            AS wasted_pct,
       COUNT(*) FILTER (WHERE is_wasted)                         AS items_with_waste,
       COUNT(*)                                                  AS items_total
FROM inventory_items
GROUP BY user_id;
