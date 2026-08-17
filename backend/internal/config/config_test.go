package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"PORT", "DATABASE_URL", "JWT_SECRET", "JWT_EXPIRY_MINUTES",
		"RATE_LIMIT_RPS", "RATE_LIMIT_BURST", "ALLOWED_ORIGIN", "SERVICE_NAME",
		"SERVICE_VERSION", "DEPLOYMENT_ENVIRONMENT", "LOG_HASH_SALT",
		"GEMINI_API_KEY", "GEMINI_BASE_URL", "GEMINI_MODEL", "GEMINI_TIMEOUT_SECONDS",
	} {
		t.Setenv(key, "")
	}

	cfg := Load()
	if cfg.Port != "8080" || cfg.JWTSecret != "dev-secret-change-me" {
		t.Errorf("unexpected core defaults: %+v", cfg)
	}
	if cfg.JWTExpiry != time.Hour || cfg.RateLimitRPS != 10 || cfg.RateLimitBurst != 20 {
		t.Errorf("unexpected duration/rate defaults: %+v", cfg)
	}
	if cfg.AllowedOrigin != "http://localhost:5173" || cfg.ServiceName != "backend-api" {
		t.Errorf("unexpected service defaults: %+v", cfg)
	}
	if cfg.GeminiModel != "gemini-2.5-flash" || cfg.GeminiTimeout != 30*time.Second {
		t.Errorf("unexpected Gemini defaults: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", "configured-secret")
	t.Setenv("JWT_EXPIRY_MINUTES", "15")
	t.Setenv("RATE_LIMIT_RPS", "2.5")
	t.Setenv("RATE_LIMIT_BURST", "7")
	t.Setenv("ALLOWED_ORIGIN", "https://app.example.com")
	t.Setenv("SERVICE_NAME", "food-api")
	t.Setenv("SERVICE_VERSION", "1.2.3")
	t.Setenv("DEPLOYMENT_ENVIRONMENT", "test")
	t.Setenv("LOG_HASH_SALT", "salt")
	t.Setenv("GEMINI_API_KEY", "key")
	t.Setenv("GEMINI_BASE_URL", "https://gemini.example.com")
	t.Setenv("GEMINI_MODEL", "test-model")
	t.Setenv("GEMINI_TIMEOUT_SECONDS", "9")

	cfg := Load()
	if cfg.Port != "9090" || cfg.DatabaseURL != "postgres://test" || cfg.JWTSecret != "configured-secret" {
		t.Errorf("core overrides not loaded: %+v", cfg)
	}
	if cfg.JWTExpiry != 15*time.Minute || cfg.RateLimitRPS != 2.5 || cfg.RateLimitBurst != 7 {
		t.Errorf("numeric overrides not loaded: %+v", cfg)
	}
	if cfg.AllowedOrigin != "https://app.example.com" || cfg.ServiceName != "food-api" || cfg.Environment != "test" {
		t.Errorf("service overrides not loaded: %+v", cfg)
	}
	if cfg.GeminiAPIKey != "key" || cfg.GeminiBaseURL != "https://gemini.example.com" ||
		cfg.GeminiModel != "test-model" || cfg.GeminiTimeout != 9*time.Second {
		t.Errorf("Gemini overrides not loaded: %+v", cfg)
	}
}

func TestInvalidNumericEnvironmentFallsBack(t *testing.T) {
	t.Setenv("TEST_INT", "not-an-int")
	t.Setenv("TEST_FLOAT", "not-a-float")
	if got := getEnvInt("TEST_INT", 42); got != 42 {
		t.Errorf("getEnvInt = %d, want 42", got)
	}
	if got := getEnvFloat("TEST_FLOAT", 3.5); got != 3.5 {
		t.Errorf("getEnvFloat = %v, want 3.5", got)
	}
}
