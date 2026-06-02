package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration, loaded from environment variables.
type Config struct {
	Port           string
	DatabaseURL    string
	JWTSecret      string
	JWTExpiry      time.Duration
	RateLimitRPS   float64
	RateLimitBurst float64
	AllowedOrigin  string
}

// Load reads configuration from the environment, applying sensible defaults
// for local development.
func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://app:app@localhost:5432/foodapp?sslmode=disable"),
		JWTSecret:      getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTExpiry:      time.Duration(getEnvInt("JWT_EXPIRY_MINUTES", 60)) * time.Minute,
		RateLimitRPS:   getEnvFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst: getEnvFloat("RATE_LIMIT_BURST", 20),
		AllowedOrigin:  getEnv("ALLOWED_ORIGIN", "http://localhost:5173"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
