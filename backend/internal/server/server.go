package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/example/food-app/backend/internal/config"
	"github.com/example/food-app/backend/internal/gemini"
	"github.com/example/food-app/backend/internal/handler"
	"github.com/example/food-app/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New builds the fully-wired HTTP handler.
func New(cfg config.Config, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Cross-cutting middleware. RequestContext and RequestLogger implement
	// the structured logging design (see observability/README.md): session
	// and user identifiers flow into every log entry, and each request
	// emits request_received / request_completed / request_failed events.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelhttp.NewMiddleware(cfg.ServiceName))
	r.Use(cors(cfg.AllowedOrigin))
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

	health := handler.NewHealthHandler(pool)
	auth := handler.NewAuthHandler(pool, cfg.JWTSecret, cfg.JWTExpiry)
	nutrients := handler.NewNutrientHandler(pool, geminiClient)
	labels := handler.NewLabelHandler(pool, geminiClient)
	nutrition := handler.NewNutritionHandler(geminiClient)
	frontendLogs := handler.NewTelemetryHandler()

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
		// to the user.
		r.With(middleware.OptionalAuthenticator(cfg.JWTSecret)).
			Post("/nutrition/analyze", nutrition.Analyze)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticator(cfg.JWTSecret))
			r.Get("/me", auth.Me)

			// AI nutrition advice, backed by the Gemini external API.
			r.Post("/nutrients/advice", nutrients.Advice)

			// Extract nutrients from a product-label image and save a food.
			r.Post("/foods/from-label", labels.ExtractAndSave)

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

// cors is a minimal CORS middleware for the local frontend origin.
func cors(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Session-Id, traceparent")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
