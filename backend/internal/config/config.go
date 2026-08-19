package config

import (
	"os"
	"strconv"
	"strings"
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
	// AllowedOrigins are the browser origins allowed to call the API. Deploying
	// the frontend separately makes this load-bearing: the production site and
	// each preview deployment are distinct origins.
	AllowedOrigins []string

	// GuestAIDailyLimit is how many AI analyses an unauthenticated visitor
	// may run per UTC day. Negative disables the cap; zero blocks guests.
	GuestAIDailyLimit int

	// Observability identity (service.name / service.version /
	// deployment.environment on every log, trace, and metric).
	ServiceName    string
	ServiceVersion string
	Environment    string
	// LogHashSalt is mixed into hashed log identifiers (client IPs and user
	// identifiers) so they cannot be reversed by brute force.
	LogHashSalt string

	// Gemini external API configuration.
	GeminiAPIKey  string
	GeminiBaseURL string
	GeminiModel   string
	GeminiTimeout time.Duration
	// GeminiAltModel is a second Gemini model for the multi-agent fan-out to
	// compare against GeminiModel. Empty leaves the fan-out single-model.
	GeminiAltModel string

	// Mistral external API configuration, a third voice in the fan-out. An
	// empty MistralAPIKey leaves the Mistral agent out.
	MistralAPIKey  string
	MistralBaseURL string
	MistralModel   string
	MistralTimeout time.Duration

	// OpenAI external API configuration, used by the multi-provider fan-out.
	// An empty OpenAIAPIKey leaves the OpenAI agent out of the fan-out.
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAITimeout time.Duration

	// LangSmith LLM tracing. An empty LangSmithAPIKey leaves it off; spans
	// still reach the OTel collector either way.
	LangSmithAPIKey  string
	LangSmithProject string
	// LangSmithTracing is LangSmith's own LANGSMITH_TRACING switch, honoured
	// so an existing .env turns the export off without removing the key.
	LangSmithTracing bool
	// LangSmithEndpoint overrides the SDK default (api.smith.langchain.com),
	// for self-hosted LangSmith.
	LangSmithEndpoint string
	// LangSmithCaptureContent attaches prompts and completions to LLM spans.
	// It only takes effect when LangSmith is enabled, because those spans also
	// go to the collector and the logging design keeps user text out of the
	// telemetry backend unless someone opts in.
	LangSmithCaptureContent bool
}

// LLMTracingEnabled reports whether LLM runs are exported to LangSmith. Both
// a key and the LANGSMITH_TRACING switch are required.
func (c Config) LLMTracingEnabled() bool {
	return c.LangSmithAPIKey != "" && c.LangSmithTracing
}

// LangSmithExportKey is the API key to export with, or "" when tracing is
// switched off — so the LANGSMITH_TRACING switch is honoured at the one place
// the key is consumed rather than at every call site.
func (c Config) LangSmithExportKey() string {
	if !c.LLMTracingEnabled() {
		return ""
	}
	return c.LangSmithAPIKey
}

// CaptureLLMContent reports whether prompt and completion text may be attached
// to spans.
func (c Config) CaptureLLMContent() bool {
	return c.LLMTracingEnabled() && c.LangSmithCaptureContent
}

// Load reads configuration from the environment, applying sensible defaults
// for local development.
func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://app:app@localhost:5433/foodapp?sslmode=disable"),
		JWTSecret:      getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTExpiry:      time.Duration(getEnvInt("JWT_EXPIRY_MINUTES", 60)) * time.Minute,
		RateLimitRPS:   getEnvFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst: getEnvFloat("RATE_LIMIT_BURST", 20),
		AllowedOrigins: splitOrigins(getEnv("ALLOWED_ORIGIN", "http://localhost:5173")),

		GuestAIDailyLimit: getEnvInt("GUEST_AI_DAILY_LIMIT", 3),

		ServiceName:    getEnv("SERVICE_NAME", "backend-api"),
		ServiceVersion: getEnv("SERVICE_VERSION", "0.1.0"),
		Environment:    getEnv("DEPLOYMENT_ENVIRONMENT", "development"),
		LogHashSalt:    getEnv("LOG_HASH_SALT", ""),
		GeminiAPIKey:   getEnv("GEMINI_API_KEY", ""),
		GeminiBaseURL:  getEnv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		GeminiModel:    getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiTimeout:  time.Duration(getEnvInt("GEMINI_TIMEOUT_SECONDS", 30)) * time.Second,
		GeminiAltModel: getEnv("GEMINI_ALT_MODEL", "gemini-3.6-flash"),

		MistralAPIKey:  getEnv("MISTRAL_API_KEY", ""),
		MistralBaseURL: getEnv("MISTRAL_BASE_URL", ""),
		MistralModel:   getEnv("MISTRAL_MODEL", ""),
		MistralTimeout: time.Duration(getEnvInt("MISTRAL_TIMEOUT_SECONDS", 60)) * time.Second,

		OpenAIAPIKey:  getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:   getEnv("OPENAI_MODEL", ""),
		OpenAITimeout: time.Duration(getEnvInt("OPENAI_TIMEOUT_SECONDS", 60)) * time.Second,

		LangSmithAPIKey:         getEnv("LANGSMITH_API_KEY", ""),
		LangSmithTracing:        getEnvBool("LANGSMITH_TRACING", true),
		LangSmithProject:        getEnv("LANGSMITH_PROJECT", "smart-food-manager"),
		LangSmithEndpoint:       getEnv("LANGSMITH_ENDPOINT", ""),
		LangSmithCaptureContent: getEnvBool("LANGSMITH_CAPTURE_CONTENT", true),
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

// splitOrigins parses a comma-separated origin list. A trailing slash is
// trimmed because an Origin header never carries one, so "https://site.com/"
// in configuration would otherwise match nothing and fail every request.
func splitOrigins(v string) []string {
	parts := strings.Split(v, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimRight(strings.TrimSpace(p), "/"); p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
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
