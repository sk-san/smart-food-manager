package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/handler"
	"github.com/sk-san/smart-food-manager/backend/internal/llm"
	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// New builds the fully-wired HTTP handler.
// panelSystemPrompt is the role every drafting agent in the panel is given.
// They share it deliberately: what should differ between drafts is the model,
// not the brief.
const panelSystemPrompt = `You are a careful nutrition assistant.
Answer in at most six sentences. Be concrete: name foods, amounts, and
timings rather than general advice. Say plainly when something is outside
what diet alone can address.`

func New(cfg config.Config, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Cross-cutting middleware. RequestContext and RequestLogger implement
	// the structured logging design (see observability/README.md): session
	// and user identifiers flow into every log entry, and each request
	// emits request_received / request_completed / request_failed events.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelhttp.NewMiddleware(cfg.ServiceName, otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true })))
	//r.Use(otelhttp.NewMiddleware(cfg.ServiceName))
	r.Use(cors(cfg.AllowedOrigins))
	r.Use(middleware.RequestContext)
	r.Use(middleware.RequestLogger("/healthz"))
	r.Use(chimw.Recoverer)
	r.Use(middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst).Middleware)

	geminiClient := gemini.New(gemini.Config{
		APIKey:  cfg.GeminiAPIKey,
		BaseURL: cfg.GeminiBaseURL,
		Model:   cfg.GeminiModel,
		Timeout: cfg.GeminiTimeout,
	})

	// Guests reach Gemini through the public analysis route, so the day's
	// allowance is capped per client IP. A signed-in caller is exempt.
	guestAI := middleware.NewGuestAIQuota(cfg.GuestAIDailyLimit)

	health := handler.NewHealthHandler(pool)
	auth := handler.NewAuthHandler(pool, cfg.JWTSecret, cfg.JWTExpiry)
	// Each feature gets its own decorator so the LangSmith run is named after
	// what the model was asked to do, not after the transport verb. The
	// handlers are unchanged: they depend on the GenerateText /
	// GenerateFromImage interfaces, which the decorator satisfies.
	recorder := tracing.NewRecorder(nil, cfg.CaptureLLMContent())
	nutrients := handler.NewNutrientHandler(pool, llm.NewTraced(geminiClient, recorder, "nutrition.advice"))
	labels := handler.NewLabelHandler(pool, llm.NewTraced(geminiClient, recorder, "label.extract"))
	nutrition := handler.NewNutritionHandler(llm.NewTraced(geminiClient, recorder, "meal.analyze"))
	// The multi-model panel is optional: without a provider key there is
	// nothing to fan out to, so the route is simply not registered rather
	// than served as an endpoint that can only fail.
	panel, panelErr := llm.NewPanel(cfg, recorder, "nutrition.panel", panelSystemPrompt)
	if panelErr != nil {
		slog.Warn("model panel disabled", "error", panelErr)
	}

	frontendLogs := handler.NewTelemetryHandler()
	meals := handler.NewMealsHandler(pool)
	goals := handler.NewGoalsHandler(pool)
	inventory := handler.NewInventoryHandler(pool)
	waste := handler.NewWasteHandler(pool)

	r.Get("/healthz", health.Healthz)

	r.Route("/api/v1", func(r chi.Router) {
		// Public endpoints.
		r.Post("/auth/login", auth.Login)
		r.Get("/nutrients", nutrients.List)

		// Frontend telemetry sink: public, but a valid token (when
		// present) binds the events to the authenticated user.
		r.With(middleware.OptionalAuthenticator(cfg.JWTSecret)).
			Post("/telemetry/logs", frontendLogs.Ingest)

		// AddEntryModal food analysis (text or image) via Gemini. Public so
		// the modal works pre-login; a token, when present, binds the call
		// to the user and lifts the guest daily limit.
		r.With(middleware.OptionalAuthenticator(cfg.JWTSecret), guestAI.Middleware).Post("/nutrition/analyze", nutrition.Analyze)
		//r.Post("/nutrition/analyze", nutrition.Analyze)

		// Remaining guest analyses, so the modal can show the allowance
		// before one is spent. Reading it never consumes a run.
		// currently ai quota is disable to conduct api tests
		r.With(middleware.OptionalAuthenticator(cfg.JWTSecret)).
			Get("/nutrition/quota", guestAI.Status)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticator(cfg.JWTSecret))
			r.Get("/me", auth.Me)
			// The account page edits one field: the display name.
			r.Patch("/me", auth.UpdateMe)

			// User-owned application data. Every query is scoped to the user id
			// from the verified JWT; resource ids alone never grant access.
			r.Route("/meals", func(r chi.Router) {
				r.Get("/", meals.List)
				r.Post("/", meals.Create)
				r.Route("/{mealID}", func(r chi.Router) {
					r.Get("/", meals.Get)
					r.Put("/", meals.Update)
					r.Delete("/", meals.Delete)
				})
			})
			r.Get("/goals", goals.Get)
			r.Put("/goals", goals.Put)
			r.Delete("/goals", goals.Delete)
			r.Route("/inventory", func(r chi.Router) {
				r.Get("/", inventory.List)
				r.Post("/", inventory.Create)
				r.Post("/scans", inventory.CreateScan)
				r.Route("/{itemID}", func(r chi.Router) {
					r.Get("/", inventory.Get)
					r.Put("/", inventory.Update)
					r.Delete("/", inventory.Delete)
					r.Post("/consume", inventory.Consume)
				})
			})
			r.Route("/waste-events", func(r chi.Router) {
				r.Get("/", waste.List)
				r.Post("/", waste.Create)
				r.Route("/{eventID}", func(r chi.Router) {
					r.Get("/", waste.Get)
					r.Put("/", waste.Update)
					r.Delete("/", waste.Delete)
				})
			})

			// AI nutrition advice, backed by the Gemini external API.
			r.Post("/nutrients/advice", nutrients.Advice)

			// Extract nutrients from a product-label image and save a food.
			r.Post("/foods/from-label", labels.ExtractAndSave)

			// The same question put to several models at once, returning the
			// merged answer with each draft beside it. Kept separate from
			// /nutrients/advice because it costs one model call per agent
			// plus the merge, where that route costs one.
			if panelErr == nil {
				r.Post("/nutrients/advice/panel", handler.NewPanelHandler(panel).Advice)
			}

			// Admin-only example (RBAC).
			r.With(middleware.RequireRole("admin")).
				Get("/admin/ping", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"admin":"pong"}`))
				})
		})
	})

	return r
}

// cors answers cross-origin requests from the configured origins.
//
// The request's Origin is matched against the allow-list and echoed back only
// on a hit. Echoing whatever arrives would let any page on the internet read
// authenticated responses on a visitor's behalf; a single fixed value, which
// this used to send, cannot cover a production site plus its preview
// deployments now that the frontend ships separately.
func cors(allowed []string) func(http.Handler) http.Handler {
	origins := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		origins[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Vary regardless of the outcome: the response differs by Origin,
			// so a shared cache must not reuse one origin's copy for another.
			w.Header().Add("Vary", "Origin")

			if origin := r.Header.Get("Origin"); origins[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Session-Id, traceparent")
				// Without this the browser hides the quota headers from the
				// frontend, which reads them to show a guest's remaining runs.
				w.Header().Set("Access-Control-Expose-Headers", "X-AI-Quota-Limit, X-AI-Quota-Remaining, X-AI-Quota-Reset")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)

			if route := chi.RouteContext(r.Context()).RoutePattern(); route != "" {
				trace.SpanFromContext(r.Context()).SetName(r.Method + " " + route)
			}
		})
	}
}
