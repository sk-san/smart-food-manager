-- Personalized targets are the only persistence concept used by the React
-- app that is not fully represented by 0001. Keep them separate from the
-- lightweight profile so every nutrient target can be updated atomically.
CREATE TABLE IF NOT EXISTS daily_goals (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    calories   NUMERIC(7,1) NOT NULL DEFAULT 2200 CHECK (calories > 0),
    protein    NUMERIC(7,2) NOT NULL DEFAULT 150  CHECK (protein > 0),
    carbs      NUMERIC(7,2) NOT NULL DEFAULT 250  CHECK (carbs > 0),
    fat        NUMERIC(7,2) NOT NULL DEFAULT 70   CHECK (fat > 0),
    sodium     NUMERIC(8,2) NOT NULL DEFAULT 2300 CHECK (sodium > 0),
    calcium    NUMERIC(8,2) NOT NULL DEFAULT 1000 CHECK (calcium > 0),
    iron       NUMERIC(7,2) NOT NULL DEFAULT 18   CHECK (iron > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Meal writes store the analyzed totals as per-100g values on a private food
-- and use a 100g meal item. These stable codes let the normalized roll-up
-- views reproduce the exact totals returned by food analysis.
INSERT INTO nutrients (code, name, unit, focus, reference_daily_amount, sort_order)
VALUES
    ('calories', 'Energy',       'kcal', 'excess_watch',     2200, 10),
    ('protein',  'Protein',      'g',    'deficiency_watch',  150, 20),
    ('carbs',    'Carbohydrate', 'g',    'excess_watch',      250, 30),
    ('fat',      'Fat',          'g',    'excess_watch',       70, 40),
    ('sodium',   'Sodium',       'mg',   'excess_watch',     2300, 50),
    ('calcium',  'Calcium',      'mg',   'deficiency_watch', 1000, 60),
    ('iron',     'Iron',         'mg',   'deficiency_watch',   18, 70)
ON CONFLICT (code) DO NOTHING;
