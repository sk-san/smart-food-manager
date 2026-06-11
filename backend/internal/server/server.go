package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/example/food-app/backend/internal/config"
	"github.com/example/food-app/backend/internal/handler"
	"github.com/example/food-app/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New builds the fully-wired HTTP handler.
func New(cfg config.Config, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Cross-cutting middleware.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelhttp.NewMiddleware("food-app"))
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors(cfg.AllowedOrigin))
	r.Use(middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst).Middleware)

	health := handler.NewHealthHandler(pool)
	auth := handler.NewAuthHandler(cfg.JWTSecret, cfg.JWTExpiry)
	nutrients := handler.NewNutrientHandler(pool)

	r.Get("/healthz", health.Healthz)

	r.Route("/api/v1", func(r chi.Router) {
		// Public endpoints.
		r.Post("/auth/login", auth.Login)
		r.Get("/nutrients", nutrients.List)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticator(cfg.JWTSecret))
			r.Get("/me", auth.Me)

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
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
