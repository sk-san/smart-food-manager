//go:build e2e

// Package e2e exercises the fully wired API over real HTTP: the actual
// chi router with every middleware, real Postgres (the docker-compose db),
// and a fake Gemini upstream so external calls stay deterministic and free.
//
// Run with:
//
//	make test-e2e            # loads .env, needs `make db-up` first
//	cd backend && go test -tags e2e -count=1 ./e2e
//
// Tests that seed rows clean up after themselves; the suite is safe to run
// against the local development database.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
	"github.com/sk-san/smart-food-manager/backend/internal/server"
	"github.com/sk-san/smart-food-manager/backend/internal/store"
)

// fakeGemini stands in for the Gemini generateContent API. Tests set the
// text of the next model reply; the server wraps it in the candidate
// envelope the real client parses.
type fakeGemini struct {
	mu   sync.Mutex
	text string
}

func (f *fakeGemini) set(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.text = text
}

func (f *fakeGemini) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		text := f.text
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": text}}}},
			},
			"usageMetadata": map[string]int{
				"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2,
			},
		})
	})
}

type suite struct {
	pool *pgxpool.Pool
	api  *httptest.Server
	fake *fakeGemini
	cfg  config.Config
}

var (
	setupOnce sync.Once
	setupErr  error
	s         *suite
)

// startSuite lazily boots the shared suite: one DB pool, one fake Gemini,
// one wired API server for the whole package. Skips (not fails) when the
// database is unreachable so `go test -tags e2e` degrades gracefully.
func startSuite(t *testing.T) *suite {
	t.Helper()
	setupOnce.Do(func() {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			dsn = "postgres://app:app@localhost:5433/foodapp?sslmode=disable"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pool, err := store.NewPool(ctx, dsn)
		if err != nil {
			setupErr = fmt.Errorf("connect %s: %w", dsn, err)
			return
		}

		fake := &fakeGemini{}
		fakeSrv := httptest.NewServer(fake.handler())

		cfg := config.Config{
			Port:           "0",
			JWTSecret:      "e2e-test-secret",
			JWTExpiry:      time.Hour,
			RateLimitRPS:   1000, // headroom so ordinary tests never trip the limiter
			RateLimitBurst: 1000,
			// Likewise for the guest AI cap: TestGuestAIDailyLimit runs its
			// own server with the production default.
			GuestAIDailyLimit: 1000,
			AllowedOrigins:    []string{"http://localhost:5173"},
			ServiceName:       "backend-api-e2e",
			ServiceVersion:    "test",
			Environment:       "test",
			GeminiAPIKey:      "e2e-fake-key",
			GeminiBaseURL:     fakeSrv.URL,
			GeminiModel:       "gemini-e2e",
			GeminiTimeout:     10 * time.Second,
		}

		s = &suite{
			pool: pool,
			api:  httptest.NewServer(server.New(cfg, pool)),
			fake: fake,
			cfg:  cfg,
		}
	})
	if setupErr != nil {
		t.Skipf("e2e environment unavailable: %v (start it with `make db-up`)", setupErr)
	}
	return s
}

// doJSON sends a request with an optional JSON body and bearer token and
// returns the status plus the decoded response body.
func (s *suite) doJSON(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	status, raw := s.doRaw(t, method, path, token, body)
	out := map[string]any{}
	if len(raw) > 0 && raw[0] == '{' {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s %s: decode body %q: %v", method, path, raw, err)
		}
	}
	return status, out
}

func (s *suite) doRaw(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.api.URL+path, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	return resp.StatusCode, raw
}

// e2ePassword is the password every seedUser row is hashed from, so tests
// can log in without needing a signup endpoint.
const e2ePassword = "integration-test-password"

// seedUser inserts a user row with a bcrypt hash of e2ePassword and removes
// it when the test finishes.
func (s *suite) seedUser(t *testing.T, email string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(e2ePassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO users (email, password_hash) VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		email, string(hash))
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM users WHERE email = $1`, email)
	})
}

// login seeds a real user row and obtains a token through the login endpoint.
func (s *suite) login(t *testing.T, email string) string {
	t.Helper()
	s.seedUser(t, email)
	status, body := s.doJSON(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"email": email, "password": e2ePassword})
	if status != http.StatusOK {
		t.Fatalf("login: got %d, want 200 (body %v)", status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("login: empty token in %v", body)
	}
	return token
}

// seedNutrient inserts a uniquely coded nutrient row and removes it (and,
// via cascade, any food_nutrients rows) when the test finishes.
func (s *suite) seedNutrient(t *testing.T) (code string, id int) {
	t.Helper()
	code = fmt.Sprintf("e2e_%d", time.Now().UnixNano())
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO nutrients (code, name, unit, focus, reference_daily_amount, sort_order, is_active)
		 VALUES ($1, 'E2E Test Nutrient', 'g', 'deficiency_watch', 50, 999, true)
		 RETURNING id`, code).Scan(&id)
	if err != nil {
		t.Fatalf("seed nutrient: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM nutrients WHERE code = $1`, code)
	})
	return code, id
}

func TestHealthz(t *testing.T) {
	s := startSuite(t)
	status, body := s.doJSON(t, http.MethodGet, "/healthz", "", nil)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %v)", status, body)
	}
	if body["db"] != true {
		t.Errorf("db = %v, want true", body["db"])
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
}

func TestLogin(t *testing.T) {
	s := startSuite(t)

	t.Run("issues a token for the seeded user", func(t *testing.T) {
		_ = s.login(t, "e2e@example.com")
	})

	t.Run("rejects a missing email", func(t *testing.T) {
		status, _ := s.doJSON(t, http.MethodPost, "/api/v1/auth/login", "",
			map[string]string{"password": "x"})
		if status != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", status)
		}
	})
}

func TestAuthAndRBAC(t *testing.T) {
	s := startSuite(t)

	t.Run("rejects /me without a token", func(t *testing.T) {
		status, _ := s.doJSON(t, http.MethodGet, "/api/v1/me", "", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", status)
		}
	})

	t.Run("rejects a garbage token", func(t *testing.T) {
		status, _ := s.doJSON(t, http.MethodGet, "/api/v1/me", "not-a-jwt", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", status)
		}
	})

	t.Run("returns claims on /me with a valid token", func(t *testing.T) {
		const email = "e2e-me@example.com"
		token := s.login(t, email)

		// The token subject is the users row id, not the address that was
		// typed into the login form, so /me reports that uuid.
		var wantID string
		if err := s.pool.QueryRow(context.Background(),
			`SELECT id::text FROM users WHERE email = $1`, email).Scan(&wantID); err != nil {
			t.Fatalf("look up seeded user: %v", err)
		}

		status, body := s.doJSON(t, http.MethodGet, "/api/v1/me", token, nil)
		if status != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %v)", status, body)
		}
		if body["user_id"] != wantID {
			t.Errorf("user_id = %v, want %s", body["user_id"], wantID)
		}
	})

	t.Run("defaults the display name to the email local part, then renames", func(t *testing.T) {
		const email = "e2e-display-name@example.com"
		token := s.login(t, email)

		// seedUser inserts no display_name, so the account starts with the
		// part of the address before the '@'.
		status, body := s.doJSON(t, http.MethodGet, "/api/v1/me", token, nil)
		if status != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %v)", status, body)
		}
		if body["display_name"] != "e2e-display-name" || body["email"] != email {
			t.Fatalf("identity = %v, want the seeded address and its local part", body)
		}

		status, body = s.doJSON(t, http.MethodPatch, "/api/v1/me", token,
			map[string]string{"display_name": "  Ada Lovelace "})
		if status != http.StatusOK {
			t.Fatalf("patch: got %d, want 200 (body %v)", status, body)
		}
		if body["display_name"] != "Ada Lovelace" {
			t.Errorf("display_name = %v, want the trimmed name", body["display_name"])
		}

		// The rename is durable, not just echoed back.
		var stored string
		if err := s.pool.QueryRow(context.Background(),
			`SELECT display_name FROM users WHERE email = $1`, email).Scan(&stored); err != nil {
			t.Fatalf("read back display name: %v", err)
		}
		if stored != "Ada Lovelace" {
			t.Errorf("stored display_name = %q, want %q", stored, "Ada Lovelace")
		}

		status, _ = s.doJSON(t, http.MethodPatch, "/api/v1/me", token,
			map[string]string{"display_name": "   "})
		if status != http.StatusBadRequest {
			t.Errorf("blank rename: got %d, want 400", status)
		}
	})

	t.Run("denies admin routes to the user role", func(t *testing.T) {
		token := s.login(t, "e2e-rbac@example.com")
		status, _ := s.doJSON(t, http.MethodGet, "/api/v1/admin/ping", token, nil)
		if status != http.StatusForbidden {
			t.Fatalf("got %d, want 403", status)
		}
	})

	t.Run("allows admin routes to the admin role", func(t *testing.T) {
		// The seeded login user only has the "user" role; mint an admin token directly
		// with the suite's signing secret to exercise RequireRole's allow path.
		token, err := middleware.NewToken(s.cfg.JWTSecret, "e2e-admin@example.com",
			[]string{"admin"}, time.Minute)
		if err != nil {
			t.Fatalf("mint admin token: %v", err)
		}
		status, body := s.doJSON(t, http.MethodGet, "/api/v1/admin/ping", token, nil)
		if status != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %v)", status, body)
		}
	})
}

func TestNutrientsList(t *testing.T) {
	s := startSuite(t)
	code, _ := s.seedNutrient(t)

	status, raw := s.doRaw(t, http.MethodGet, "/api/v1/nutrients", "", nil)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", status, raw)
	}
	var list []struct {
		Code string `json:"code"`
		Unit string `json:"unit"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode list: %v (body %s)", err, raw)
	}
	for _, n := range list {
		if n.Code == code {
			if n.Unit != "g" {
				t.Errorf("seeded nutrient unit = %q, want g", n.Unit)
			}
			return
		}
	}
	t.Fatalf("seeded nutrient %q not in list of %d rows", code, len(list))
}

func TestNutritionAnalyze(t *testing.T) {
	s := startSuite(t)

	t.Run("returns items for a text description", func(t *testing.T) {
		s.fake.set(`{"items":[{"name":"Grilled salmon","calories":230,"protein":25,` +
			`"carbs":0,"fat":14,"sodium":60,"calcium":15,"iron":0.5}]}`)
		status, raw := s.doRaw(t, http.MethodPost, "/api/v1/nutrition/analyze", "",
			map[string]string{"type": "text", "text": "grilled salmon fillet"})
		if status != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %s)", status, raw)
		}
		var items []struct {
			Name     string  `json:"name"`
			Calories float64 `json:"calories"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatalf("decode items: %v (body %s)", err, raw)
		}
		if len(items) != 1 || items[0].Name != "Grilled salmon" || items[0].Calories != 230 {
			t.Fatalf("items = %+v, want one Grilled salmon @230kcal", items)
		}
	})

	t.Run("rejects an unknown payload type", func(t *testing.T) {
		status, _ := s.doJSON(t, http.MethodPost, "/api/v1/nutrition/analyze", "",
			map[string]string{"type": "audio"})
		if status != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", status)
		}
	})

	t.Run("rejects blank text", func(t *testing.T) {
		status, _ := s.doJSON(t, http.MethodPost, "/api/v1/nutrition/analyze", "",
			map[string]string{"type": "text", "text": "   "})
		if status != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", status)
		}
	})

	t.Run("maps an unparseable model reply to 502", func(t *testing.T) {
		s.fake.set("sorry, I can only speak prose")
		status, _ := s.doJSON(t, http.MethodPost, "/api/v1/nutrition/analyze", "",
			map[string]string{"type": "text", "text": "mystery stew"})
		if status != http.StatusBadGateway {
			t.Fatalf("got %d, want 502", status)
		}
	})
}

// TestGuestAIDailyLimit exercises the guest cap over real HTTP against a
// server configured with the production default of three analyses a day. It
// needs its own server because the cap counts per process and the shared
// suite deliberately runs with headroom.
func TestGuestAIDailyLimit(t *testing.T) {
	s := startSuite(t)

	fake := &fakeGemini{}
	fake.set(`{"items":[{"name":"Apple","calories":95,"protein":0.5,"carbs":25,` +
		`"fat":0.3,"sodium":1,"calcium":6,"iron":0.1}]}`)
	fakeSrv := httptest.NewServer(fake.handler())
	defer fakeSrv.Close()

	cfg := s.cfg
	cfg.GeminiBaseURL = fakeSrv.URL
	cfg.GuestAIDailyLimit = 3
	capped := &suite{pool: s.pool, api: httptest.NewServer(server.New(cfg, s.pool)), fake: fake, cfg: cfg}
	defer capped.api.Close()

	analyze := func(t *testing.T, token string) (int, map[string]any) {
		t.Helper()
		return capped.doJSON(t, http.MethodPost, "/api/v1/nutrition/analyze", token,
			map[string]string{"type": "text", "text": "an apple"})
	}

	t.Run("a guest gets three analyses", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			if status, body := analyze(t, ""); status != http.StatusOK {
				t.Fatalf("analysis %d: got %d, want 200 (body %v)", i, status, body)
			}
		}
		status, quota := capped.doJSON(t, http.MethodGet, "/api/v1/nutrition/quota", "", nil)
		if status != http.StatusOK {
			t.Fatalf("quota: got %d, want 200", status)
		}
		if quota["remaining"] != float64(0) || quota["limit"] != float64(3) {
			t.Fatalf("quota = %v, want limit 3 and remaining 0", quota)
		}
	})

	t.Run("the fourth is rejected with the quota code", func(t *testing.T) {
		status, body := analyze(t, "")
		if status != http.StatusTooManyRequests {
			t.Fatalf("got %d, want 429 (body %v)", status, body)
		}
		if body["code"] != middleware.GuestAIQuotaCode {
			t.Errorf("code = %v, want %q", body["code"], middleware.GuestAIQuotaCode)
		}
	})

	t.Run("a signed-in user is not capped", func(t *testing.T) {
		token := capped.login(t, "guest-quota@example.com")
		for i := 1; i <= 4; i++ {
			if status, body := analyze(t, token); status != http.StatusOK {
				t.Fatalf("authenticated analysis %d: got %d, want 200 (body %v)", i, status, body)
			}
		}
	})
}

func TestAdvice(t *testing.T) {
	s := startSuite(t)

	t.Run("requires authentication", func(t *testing.T) {
		status, _ := s.doJSON(t, http.MethodPost, "/api/v1/nutrients/advice", "",
			map[string]string{"prompt": "how much protein per day?"})
		if status != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", status)
		}
	})

	t.Run("returns the model's advice", func(t *testing.T) {
		token := s.login(t, "e2e-advice@example.com")
		s.fake.set(`{"advice":"Aim for roughly 0.8 g of protein per kg of body weight."}`)
		status, body := s.doJSON(t, http.MethodPost, "/api/v1/nutrients/advice", token,
			map[string]string{"prompt": "how much protein per day?"})
		if status != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %v)", status, body)
		}
		advice, _ := body["advice"].(string)
		if !strings.Contains(advice, "0.8 g of protein") {
			t.Fatalf("advice = %q, want the fake model text", advice)
		}
	})

	t.Run("maps malformed model output to 502", func(t *testing.T) {
		token := s.login(t, "e2e-advice-malformed@example.com")
		s.fake.set("this is not JSON")
		status, body := s.doJSON(t, http.MethodPost, "/api/v1/nutrients/advice", token,
			map[string]string{"prompt": "what should I eat?"})
		if status != http.StatusBadGateway || body["error"] != "could not parse advice" {
			t.Fatalf("got status=%d body=%v, want 502 parse error", status, body)
		}
	})

	t.Run("maps empty model advice to 502", func(t *testing.T) {
		token := s.login(t, "e2e-advice-empty@example.com")
		s.fake.set(`{"advice":"   "}`)
		status, body := s.doJSON(t, http.MethodPost, "/api/v1/nutrients/advice", token,
			map[string]string{"prompt": "what should I eat?"})
		if status != http.StatusBadGateway || body["error"] != "advice result is empty" {
			t.Fatalf("got status=%d body=%v, want 502 empty-advice error", status, body)
		}
	})
}

func TestLabelExtractAndSave(t *testing.T) {
	s := startSuite(t)
	code, _ := s.seedNutrient(t)
	token := s.login(t, "e2e-label@example.com")

	// The model reply references one storable code and one bogus code, so
	// the handler must save the first and report the second as skipped.
	s.fake.set(fmt.Sprintf(`{"name":"E2E Protein Bar","food_type":"prepared_food",`+
		`"category":"snack","nutrients":[{"code":%q,"amount_per_100g":12.5},`+
		`{"code":"e2e_bogus_code","amount_per_100g":1}]}`, code))

	// Real multipart upload with an explicit image/png part header (the
	// handler rejects the default application/octet-stream).
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="image"; filename="label.png"`)
	hdr.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(img.Bytes()); err != nil {
		t.Fatalf("write image: %v", err)
	}
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, s.api.URL+"/api/v1/foods/from-label", &form)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		FoodID         string   `json:"food_id"`
		Name           string   `json:"name"`
		SavedNutrients int      `json:"saved_nutrients"`
		SkippedCodes   []string `json:"skipped_codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201 (%+v)", resp.StatusCode, out)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM foods WHERE id = $1`, out.FoodID)
	})

	if out.Name != "E2E Protein Bar" || out.SavedNutrients != 1 {
		t.Errorf("response = %+v, want E2E Protein Bar with 1 saved nutrient", out)
	}
	if len(out.SkippedCodes) != 1 || out.SkippedCodes[0] != "e2e_bogus_code" {
		t.Errorf("skipped = %v, want [e2e_bogus_code]", out.SkippedCodes)
	}

	// The food and its per-100g amount must actually be in Postgres.
	var amount float64
	err = s.pool.QueryRow(context.Background(),
		`SELECT fn.amount_per_100g
		   FROM food_nutrients fn
		   JOIN nutrients n ON n.id = fn.nutrient_id
		  WHERE fn.food_id = $1 AND n.code = $2`, out.FoodID, code).Scan(&amount)
	if err != nil {
		t.Fatalf("food_nutrients row missing: %v", err)
	}
	if amount != 12.5 {
		t.Errorf("amount_per_100g = %v, want 12.5", amount)
	}
}

func TestAuthenticatedPersistenceCRUD(t *testing.T) {
	s := startSuite(t)
	token := s.login(t, "e2e-persistence@example.com")

	t.Run("goals", func(t *testing.T) {
		goals := map[string]float64{
			"calories": 2400, "protein": 160, "carbs": 275, "fat": 75,
			"sodium": 2200, "calcium": 1100, "iron": 20,
		}
		status, body := s.doJSON(t, http.MethodPut, "/api/v1/goals", token, goals)
		if status != http.StatusOK || body["calories"] != float64(2400) {
			t.Fatalf("put goals: status=%d body=%v", status, body)
		}
		status, body = s.doJSON(t, http.MethodGet, "/api/v1/goals", token, nil)
		if status != http.StatusOK || body["protein"] != float64(160) {
			t.Fatalf("get goals: status=%d body=%v", status, body)
		}
		status, _ = s.doJSON(t, http.MethodDelete, "/api/v1/goals", token, nil)
		if status != http.StatusNoContent {
			t.Fatalf("delete goals: got %d, want 204", status)
		}
	})

	var mealID string
	t.Run("meals", func(t *testing.T) {
		meal := map[string]any{
			"name": "E2E lentil bowl", "consumed_at": "2026-08-11T12:30:00Z",
			"calories": 510, "protein": 28, "carbs": 72, "fat": 13,
			"sodium": 640, "calcium": 120, "iron": 6.5,
		}
		status, body := s.doJSON(t, http.MethodPost, "/api/v1/meals", token, meal)
		if status != http.StatusCreated {
			t.Fatalf("create meal: status=%d body=%v", status, body)
		}
		mealID, _ = body["id"].(string)
		if mealID == "" || body["calories"] != float64(510) {
			t.Fatalf("created meal = %v", body)
		}
		otherToken := s.login(t, "e2e-persistence-other@example.com")
		status, _ = s.doJSON(t, http.MethodGet, "/api/v1/meals/"+mealID, otherToken, nil)
		if status != http.StatusNotFound {
			t.Fatalf("another user read the meal: got %d, want 404", status)
		}

		status, raw := s.doRaw(t, http.MethodGet, "/api/v1/meals", token, nil)
		var meals []map[string]any
		if status != http.StatusOK || json.Unmarshal(raw, &meals) != nil || len(meals) != 1 {
			t.Fatalf("list meals: status=%d body=%s", status, raw)
		}

		meal["name"] = "Updated lentil bowl"
		meal["calories"] = 525
		status, body = s.doJSON(t, http.MethodPut, "/api/v1/meals/"+mealID, token, meal)
		if status != http.StatusOK || body["name"] != "Updated lentil bowl" {
			t.Fatalf("update meal: status=%d body=%v", status, body)
		}

		status, _ = s.doJSON(t, http.MethodDelete, "/api/v1/meals/"+mealID, token, nil)
		if status != http.StatusNoContent {
			t.Fatalf("delete meal: got %d, want 204", status)
		}
	})

	t.Run("inventory and waste stay in sync", func(t *testing.T) {
		status, body := s.doJSON(t, http.MethodPost, "/api/v1/inventory", token, map[string]any{
			"name": "Already expired", "quantity_purchased": 100,
			"quantity_consumed": 0, "best_before_date": "2000-01-01",
			"date_label": "best_before", "storage": "pantry", "package": "unopened",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("create expired inventory: status=%d body=%v", status, body)
		}

		item := map[string]any{
			"name": "E2E spinach", "quantity_purchased": 300,
			"quantity_consumed": 50, "best_before_date": "2099-08-15",
			"date_label": "best_before", "storage": "fridge", "package": "opened",
		}
		status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory", token, item)
		if status != http.StatusCreated {
			t.Fatalf("create inventory: status=%d body=%v", status, body)
		}
		itemID, _ := body["id"].(string)
		if itemID == "" {
			t.Fatalf("created inventory has no id: %v", body)
		}
		item["best_before_date"] = "2000-01-01"
		status, body = s.doJSON(t, http.MethodPut, "/api/v1/inventory/"+itemID, token, item)
		if status != http.StatusBadRequest {
			t.Fatalf("update inventory to expired date: status=%d body=%v", status, body)
		}
		item["best_before_date"] = "2099-08-15"
		item["name"] = "E2E baby spinach"
		item["quantity_purchased"] = 320
		status, body = s.doJSON(t, http.MethodPut, "/api/v1/inventory/"+itemID, token, item)
		if status != http.StatusOK || body["name"] != "E2E baby spinach" {
			t.Fatalf("update inventory: status=%d body=%v", status, body)
		}

		waste := map[string]any{
			"inventory_item_id": itemID, "quantity_g": 40,
			"reason": "spoiled_visible", "spoilage": "visual_mold",
		}
		status, body = s.doJSON(t, http.MethodPost, "/api/v1/waste-events", token, waste)
		if status != http.StatusCreated {
			t.Fatalf("create waste: status=%d body=%v", status, body)
		}
		eventID, _ := body["id"].(string)

		status, body = s.doJSON(t, http.MethodGet, "/api/v1/inventory/"+itemID, token, nil)
		if status != http.StatusOK || body["quantity_wasted"] != float64(40) {
			t.Fatalf("inventory after waste: status=%d body=%v", status, body)
		}

		waste["quantity_g"] = 60
		status, body = s.doJSON(t, http.MethodPut, "/api/v1/waste-events/"+eventID, token, waste)
		if status != http.StatusOK || body["quantity_g"] != float64(60) {
			t.Fatalf("update waste: status=%d body=%v", status, body)
		}

		status, _ = s.doJSON(t, http.MethodDelete, "/api/v1/waste-events/"+eventID, token, nil)
		if status != http.StatusNoContent {
			t.Fatalf("delete waste: got %d, want 204", status)
		}
		status, body = s.doJSON(t, http.MethodGet, "/api/v1/inventory/"+itemID, token, nil)
		if status != http.StatusOK || body["quantity_wasted"] != float64(0) {
			t.Fatalf("inventory after waste delete: status=%d body=%v", status, body)
		}
		status, body = s.doJSON(t, http.MethodPost, "/api/v1/waste-events", token,
			map[string]any{
				"inventory_item_id": itemID, "quantity_g": 10,
				"reason": "overbought", "date_label": "best_before", "package": "opened",
			})
		if status != http.StatusCreated {
			t.Fatalf("create retained waste: status=%d body=%v", status, body)
		}
		retainedEventID, _ := body["id"].(string)
		status, _ = s.doJSON(t, http.MethodDelete, "/api/v1/inventory/"+itemID, token, nil)
		if status != http.StatusNoContent {
			t.Fatalf("delete inventory: got %d, want 204", status)
		}
		status, body = s.doJSON(t, http.MethodGet, "/api/v1/waste-events/"+retainedEventID, token, nil)
		if status != http.StatusOK || body["inventory_item_id"] != nil {
			t.Fatalf("retained waste after inventory delete: status=%d body=%v", status, body)
		}
	})
}

func TestScannedInventoryLifecycle(t *testing.T) {
	s := startSuite(t)
	token := s.login(t, "e2e-scanned-inventory@example.com")

	scan := map[string]any{
		"source_type": "product", "name": "E2E beef snack", "category": "beef",
		"quantity_g": 50, "expiry_date": "2099-09-01", "expiry_is_estimated": false,
		"date_label": "best_before", "storage": "pantry", "package": "unopened",
		"consumed_at": "2026-08-16T09:00:00Z",
		"nutrients": map[string]float64{
			"calories": 200, "protein": 10, "carbs": 15, "fat": 8,
			"sodium": 120, "calcium": 20, "iron": 1,
		},
	}
	status, body := s.doJSON(t, http.MethodPost, "/api/v1/inventory/scans", token, scan)
	if status != http.StatusCreated {
		t.Fatalf("scan product: status=%d body=%v", status, body)
	}
	inventory, _ := body["inventory"].(map[string]any)
	meal, _ := body["meal"].(map[string]any)
	itemID, _ := inventory["id"].(string)
	provisionalID, _ := inventory["provisional_meal_id"].(string)
	nutrition, _ := inventory["nutrition_per_100g"].(map[string]any)
	if itemID == "" || provisionalID == "" || inventory["source_type"] != "product" {
		t.Fatalf("scanned inventory = %v", inventory)
	}
	if nutrition["calories"] != float64(400) || meal["calories"] != float64(200) {
		t.Fatalf("normalized nutrition=%v provisional meal=%v", nutrition, meal)
	}
	status, body = s.doJSON(t, http.MethodPut, "/api/v1/meals/"+provisionalID, token,
		map[string]any{
			"name": "Edited provisional", "consumed_at": "2026-08-16T09:00:00Z",
			"calories": 1, "protein": 1, "carbs": 1, "fat": 1,
			"sodium": 1, "calcium": 1, "iron": 1,
		})
	if status != http.StatusConflict {
		t.Fatalf("update provisional meal: status=%d body=%v", status, body)
	}
	status, body = s.doJSON(t, http.MethodDelete, "/api/v1/meals/"+provisionalID, token, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete provisional meal: status=%d body=%v", status, body)
	}
	status, body = s.doJSON(t, http.MethodPut, "/api/v1/inventory/"+itemID, token,
		map[string]any{
			"name": "E2E beef snack", "quantity_purchased": 50, "quantity_consumed": 1,
			"best_before_date": "2099-09-01", "date_label": "best_before",
			"storage": "pantry", "package": "unopened",
		})
	if status != http.StatusConflict {
		t.Fatalf("direct scanned consumption update: status=%d body=%v", status, body)
	}
	status, manualWaste := s.doJSON(t, http.MethodPost, "/api/v1/waste-events", token,
		map[string]any{
			"inventory_item_id": itemID, "quantity_g": 5,
			"reason": "overbought", "date_label": "best_before", "package": "unopened",
		})
	if status != http.StatusCreated || manualWaste["quantity_g"] != float64(5) {
		t.Fatalf("manual provisional waste: status=%d body=%v", status, manualWaste)
	}
	status, resizedProvisional := s.doJSON(t, http.MethodGet, "/api/v1/meals/"+provisionalID, token, nil)
	if status != http.StatusOK || resizedProvisional["calories"] != float64(180) {
		t.Fatalf("provisional after manual waste: status=%d body=%v", status, resizedProvisional)
	}

	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/"+itemID+"/consume", token,
		map[string]any{"quantity_g": 20, "discard_remaining": false})
	if status != http.StatusOK {
		t.Fatalf("first consume: status=%d body=%v", status, body)
	}
	inventory, _ = body["inventory"].(map[string]any)
	meal, _ = body["meal"].(map[string]any)
	if inventory["quantity_consumed"] != float64(20) || inventory["provisional_meal_id"] != nil {
		t.Fatalf("finalized inventory = %v", inventory)
	}
	if meal["id"] != provisionalID || meal["calories"] != float64(80) {
		t.Fatalf("finalized provisional meal = %v", meal)
	}

	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/"+itemID+"/consume", token,
		map[string]any{"quantity_g": 10, "discard_remaining": true, "waste_reason": "overbought"})
	if status != http.StatusOK {
		t.Fatalf("consume and discard: status=%d body=%v", status, body)
	}
	inventory, _ = body["inventory"].(map[string]any)
	meal, _ = body["meal"].(map[string]any)
	waste, _ := body["waste_event"].(map[string]any)
	if inventory["is_resolved"] != true || inventory["quantity_wasted"] != float64(20) {
		t.Fatalf("resolved inventory = %v", inventory)
	}
	if meal["id"] == provisionalID || meal["calories"] != float64(40) {
		t.Fatalf("incremental meal = %v", meal)
	}
	if waste["quantity_g"] != float64(15) || waste["impact_kg_co2e"].(float64) <= 0 ||
		waste["virtual_water_l"].(float64) <= 0 || waste["tree_equivalents"].(float64) <= 0 {
		t.Fatalf("waste impact = %v", waste)
	}

	freshIngredientScan := map[string]any{
		"source_type": "ingredient", "name": "E2E oats", "category": "grains",
		"quantity_g": 100, "expiry_date": "2099-10-01", "expiry_is_estimated": true,
		"date_label": "best_before", "storage": "pantry", "package": "unopened",
		"nutrients": map[string]float64{
			"calories": 300, "protein": 12, "carbs": 55, "fat": 5,
			"sodium": 2, "calcium": 40, "iron": 4,
		},
	}
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/scans", token, freshIngredientScan)
	if status != http.StatusCreated || body["meal"] != nil {
		t.Fatalf("scan fresh ingredient: status=%d body=%v", status, body)
	}
	freshIngredient, _ := body["inventory"].(map[string]any)
	freshIngredientID, _ := freshIngredient["id"].(string)
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/"+freshIngredientID+"/consume", token,
		map[string]any{"quantity_g": 100, "discard_remaining": false})
	if status != http.StatusOK {
		t.Fatalf("consume ingredient: status=%d body=%v", status, body)
	}
	freshIngredient, _ = body["inventory"].(map[string]any)
	meal, _ = body["meal"].(map[string]any)
	if freshIngredient["is_resolved"] != true || meal["calories"] != float64(300) {
		t.Fatalf("ingredient consumption inventory=%v meal=%v", freshIngredient, meal)
	}

	deletableScan := map[string]any{
		"source_type": "food", "name": "E2E deletable meal", "category": "prepared",
		"quantity_g": 25, "expiry_date": "2099-10-01", "expiry_is_estimated": false,
		"date_label": "best_before", "storage": "fridge", "package": "opened",
		"nutrients": map[string]float64{"calories": 100},
	}
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/scans", token, deletableScan)
	if status != http.StatusCreated {
		t.Fatalf("scan deletable food: status=%d body=%v", status, body)
	}
	deletableInventory, _ := body["inventory"].(map[string]any)
	deletableMeal, _ := body["meal"].(map[string]any)
	deletableItemID, _ := deletableInventory["id"].(string)
	deletableMealID, _ := deletableMeal["id"].(string)
	status, _ = s.doJSON(t, http.MethodDelete, "/api/v1/inventory/"+deletableItemID, token, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete scanned food: got %d, want 204", status)
	}
	status, _ = s.doJSON(t, http.MethodGet, "/api/v1/meals/"+deletableMealID, token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("provisional meal survived inventory delete: got %d, want 404", status)
	}

	ingredientScan := map[string]any{
		"source_type": "ingredient", "name": "E2E expired spinach", "category": "vegetable",
		"quantity_g": 100, "expiry_date": "2000-01-01", "expiry_is_estimated": true,
		"date_label": "use_by", "storage": "fridge", "package": "unopened",
		"nutrients": map[string]float64{
			"calories": 23, "protein": 2.9, "carbs": 3.6, "fat": 0.4,
			"sodium": 79, "calcium": 99, "iron": 2.7,
		},
	}
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/scans", token, ingredientScan)
	if status != http.StatusBadRequest {
		t.Fatalf("scan past-expiry ingredient: status=%d body=%v", status, body)
	}
	ingredientScan["expiry_date"] = "2099-10-01"
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/scans", token, ingredientScan)
	if status != http.StatusCreated || body["meal"] != nil {
		t.Fatalf("scan ingredient for expiry race: status=%d body=%v", status, body)
	}
	expiringIngredient, _ := body["inventory"].(map[string]any)
	expiringIngredientID, _ := expiringIngredient["id"].(string)
	if _, err := s.pool.Exec(context.Background(), `
		UPDATE inventory_items SET use_by_date = '2000-01-01'
		WHERE id = $1`, expiringIngredientID); err != nil {
		t.Fatalf("force ingredient expiry: %v", err)
	}
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/"+expiringIngredientID+"/consume", token,
		map[string]any{"quantity_g": 10, "discard_remaining": false})
	if status != http.StatusOK {
		t.Fatalf("consume newly expired ingredient: status=%d body=%v", status, body)
	}
	expiredInventory, _ := body["inventory"].(map[string]any)
	expiredWaste, _ := body["waste_event"].(map[string]any)
	if expiredInventory["is_resolved"] != true || expiredInventory["quantity_wasted"] != float64(100) ||
		expiredInventory["quantity_consumed"] != float64(0) || expiredWaste["quantity_g"] != float64(100) {
		t.Fatalf("expired consume reconciliation: status=%d body=%v", status, body)
	}
	status, _ = s.doJSON(t, http.MethodGet, "/api/v1/inventory/"+expiringIngredientID, token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("resolved expired inventory get: got %d, want 404", status)
	}
	expiredWasteID, _ := expiredWaste["id"].(string)
	status, body = s.doJSON(t, http.MethodPut, "/api/v1/waste-events/"+expiredWasteID, token,
		map[string]any{
			"inventory_item_id": expiringIngredientID, "quantity_g": 50,
			"reason": "expired_use_by", "date_label": "use_by",
			"date_status": "15_plus_days_after", "package": "unopened",
		})
	if status != http.StatusConflict {
		t.Fatalf("update automatic expiry waste: status=%d body=%v", status, body)
	}
	status, body = s.doJSON(t, http.MethodDelete, "/api/v1/waste-events/"+expiredWasteID, token, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete automatic expiry waste: status=%d body=%v", status, body)
	}

	autoExpiryScan := map[string]any{
		"source_type": "product", "name": "E2E expiring product", "category": "prepared",
		"quantity_g": 40, "expiry_date": "2099-10-01", "expiry_is_estimated": true,
		"date_label": "best_before", "storage": "fridge", "package": "unopened",
		"nutrients": map[string]float64{"calories": 160, "protein": 4},
	}
	status, body = s.doJSON(t, http.MethodPost, "/api/v1/inventory/scans", token, autoExpiryScan)
	if status != http.StatusCreated {
		t.Fatalf("scan product for auto-expiry: status=%d body=%v", status, body)
	}
	autoInventory, _ := body["inventory"].(map[string]any)
	autoMeal, _ := body["meal"].(map[string]any)
	autoItemID, _ := autoInventory["id"].(string)
	autoMealID, _ := autoMeal["id"].(string)
	status, autoManualWaste := s.doJSON(t, http.MethodPost, "/api/v1/waste-events", token,
		map[string]any{
			"inventory_item_id": autoItemID, "quantity_g": 5,
			"reason": "overbought", "date_label": "best_before", "package": "unopened",
		})
	if status != http.StatusCreated {
		t.Fatalf("manual waste before product expiry: status=%d body=%v", status, autoManualWaste)
	}
	autoManualWasteID, _ := autoManualWaste["id"].(string)
	if _, err := s.pool.Exec(context.Background(), `
		UPDATE inventory_items SET best_before_date = '2000-01-01'
		WHERE id = $1`, autoItemID); err != nil {
		t.Fatalf("force product expiry: %v", err)
	}

	status, raw := s.doRaw(t, http.MethodGet, "/api/v1/inventory", token, nil)
	var openInventory []map[string]any
	if status != http.StatusOK || json.Unmarshal(raw, &openInventory) != nil || len(openInventory) != 0 {
		t.Fatalf("open inventory after expiry: status=%d body=%s", status, raw)
	}
	status, _ = s.doJSON(t, http.MethodGet, "/api/v1/meals/"+autoMealID, token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("auto-expired provisional meal survived: got %d, want 404", status)
	}
	status, body = s.doJSON(t, http.MethodPut, "/api/v1/waste-events/"+autoManualWasteID, token,
		map[string]any{
			"inventory_item_id": autoItemID, "quantity_g": 2,
			"reason": "overbought", "date_label": "best_before", "package": "unopened",
		})
	if status != http.StatusConflict {
		t.Fatalf("reduce manual waste after expiry: status=%d body=%v", status, body)
	}
	status, body = s.doJSON(t, http.MethodDelete, "/api/v1/waste-events/"+autoManualWasteID, token, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete manual waste after expiry: status=%d body=%v", status, body)
	}
	status, raw = s.doRaw(t, http.MethodGet, "/api/v1/waste-events", token, nil)
	var wasteEvents []map[string]any
	if status != http.StatusOK || json.Unmarshal(raw, &wasteEvents) != nil || len(wasteEvents) != 5 {
		t.Fatalf("waste events after expiry: status=%d body=%s", status, raw)
	}
	status, raw = s.doRaw(t, http.MethodGet, "/api/v1/waste-events", token, nil)
	var secondRead []map[string]any
	if status != http.StatusOK || json.Unmarshal(raw, &secondRead) != nil || len(secondRead) != len(wasteEvents) {
		t.Fatalf("idempotent expiry read: status=%d body=%s", status, raw)
	}
}

func TestTelemetryIngest(t *testing.T) {
	s := startSuite(t)
	status, body := s.doJSON(t, http.MethodPost, "/api/v1/telemetry/logs", "",
		map[string]any{"events": []map[string]any{
			{
				"event.name":    "screen_view",
				"severity":      "INFO",
				"message":       "viewed dashboard",
				"event.action":  "view",
				"event.outcome": "success",
			},
			{"event.name": "not_in_catalog", "severity": "INFO",
				"message": "x", "event.outcome": "success"},
		}})
	if status != http.StatusAccepted {
		t.Fatalf("got %d, want 202 (body %v)", status, body)
	}
	if body["accepted"] != float64(1) || body["dropped"] != float64(1) {
		t.Fatalf("counts = %v, want accepted 1 / dropped 1", body)
	}
}

func TestCORSPreflight(t *testing.T) {
	s := startSuite(t)
	req, err := http.NewRequest(http.MethodOptions, s.api.URL+"/api/v1/nutrients", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	// A browser preflight always carries Origin, and the API now answers only
	// the origins it recognises rather than echoing a fixed value.
	req.Header.Set("Origin", s.cfg.AllowedOrigins[0])

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != s.cfg.AllowedOrigins[0] {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, s.cfg.AllowedOrigins[0])
	}

	// An unlisted origin must get no grant, or deploying the frontend on its
	// own domain would still leave the API open to every other site.
	unlisted, err := http.NewRequest(http.MethodOptions, s.api.URL+"/api/v1/nutrients", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	unlisted.Header.Set("Origin", "https://evil.example.com")

	res, err := http.DefaultClient.Do(unlisted)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for an unlisted origin, want none", got)
	}
}

func TestRateLimit(t *testing.T) {
	s := startSuite(t)

	// A dedicated server with a tiny bucket so the shared suite (which uses
	// generous limits) is unaffected.
	cfg := s.cfg
	cfg.RateLimitRPS = 1
	cfg.RateLimitBurst = 2
	limited := httptest.NewServer(server.New(cfg, s.pool))
	defer limited.Close()

	var ok, throttled int
	for i := 0; i < 6; i++ {
		resp, err := http.Get(limited.URL + "/healthz")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			throttled++
		default:
			t.Fatalf("request %d: unexpected status %d", i, resp.StatusCode)
		}
	}
	if ok == 0 || throttled == 0 {
		t.Fatalf("ok=%d throttled=%d, want both burst passes and 429s", ok, throttled)
	}
}
