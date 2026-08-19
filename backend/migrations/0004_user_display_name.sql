-- Seed the account display name.  users.display_name has existed since 0001 but
-- nothing ever wrote to it, so every account carries a NULL.  Give each one the
-- local part of its address (everything before '@') — the same starting point a
-- row inserted without a name gets from the read-time fallback in
-- backend/internal/handler/auth.go.  From here the user owns the value and edits
-- it on the account page.
UPDATE users
SET    display_name = split_part(email::text, '@', 1),
       updated_at   = now()
WHERE  display_name IS NULL OR btrim(display_name) = '';
