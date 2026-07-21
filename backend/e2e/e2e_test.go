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

	"github.com/example/food-app/backend/internal/config"
	"github.com/example/food-app/backend/internal/middleware"
	"github.com/example/food-app/backend/internal/server"
	"github.com/example/food-app/backend/internal/store"
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
			AllowedOrigin:  "http://localhost:5173",
			ServiceName:    "backend-api-e2e",
			ServiceVersion: "test",
			Environment:    "test",
			GeminiAPIKey:   "e2e-fake-key",
			GeminiBaseURL:  fakeSrv.URL,
			GeminiModel:    "gemini-e2e",
			GeminiTimeout:  10 * time.Second,
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
const e2ePassword = "irrelevant-demo-password"

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

	t.Run("issues a token for the demo user", func(t *testing.T) {
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
		token := s.login(t, "e2e-me@example.com")
		status, body := s.doJSON(t, http.MethodGet, "/api/v1/me", token, nil)
		if status != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %v)", status, body)
		}
		if body["user_id"] != "e2e-me@example.com" {
			t.Errorf("user_id = %v, want e2e-me@example.com", body["user_id"])
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
		// The demo login only issues "user"; mint an admin token directly
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
		s.fake.set("Aim for roughly 0.8 g of protein per kg of body weight.")
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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != s.cfg.AllowedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, s.cfg.AllowedOrigin)
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
