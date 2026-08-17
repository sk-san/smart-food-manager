-- Connect scanned pantry stock to its nutrition/intake lifecycle.  The
-- provisional meal represents the scan-time assumption that a prepared food
-- or product will be fully consumed; the first explicit consume action
-- finalizes (or removes) that meal and clears the link.
ALTER TABLE inventory_items
    ADD COLUMN source_type TEXT NOT NULL DEFAULT 'ingredient'
        CHECK (source_type IN ('food', 'product', 'ingredient')),
    ADD COLUMN provisional_meal_id UUID REFERENCES meals(id) ON DELETE SET NULL,
    ADD COLUMN expiry_is_estimated BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_inventory_expiry_unresolved
    ON inventory_items (user_id, COALESCE(use_by_date, best_before_date))
    WHERE is_resolved = FALSE;

CREATE INDEX idx_inventory_provisional_meal
    ON inventory_items (provisional_meal_id)
    WHERE provisional_meal_id IS NOT NULL;
