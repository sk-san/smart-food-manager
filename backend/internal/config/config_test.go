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
		"GEMINI_ALT_MODEL",
		"LANGSMITH_API_KEY", "LANGSMITH_PROJECT", "LANGSMITH_ENDPOINT",
		"LANGSMITH_CAPTURE_CONTENT", "OPENAI_API_KEY", "OPENAI_MODEL",
		"OPENAI_TIMEOUT_SECONDS", "MISTRAL_API_KEY", "MISTRAL_BASE_URL",
		"MISTRAL_MODEL", "MISTRAL_TIMEOUT_SECONDS",
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
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "http://localhost:5173" ||
		cfg.ServiceName != "backend-api" {
		t.Errorf("unexpected service defaults: %+v", cfg)
	}
	if cfg.GeminiModel != "gemini-2.5-flash" || cfg.GeminiTimeout != 60*time.Second {
		t.Errorf("unexpected Gemini defaults: %+v", cfg)
	}
	// The fan-out's second model must differ from the first, or the two agents
	// would be the same model twice.
	if cfg.GeminiAltModel == "" || cfg.GeminiAltModel == cfg.GeminiModel {
		t.Errorf("GeminiAltModel = %q, want a distinct second model", cfg.GeminiAltModel)
	}
	// LLM tracing stays off until a key is supplied, so the default build
	// exports spans to the collector alone.
	if cfg.LLMTracingEnabled() || cfg.CaptureLLMContent() {
		t.Errorf("LangSmith tracing enabled without an API key: %+v", cfg)
	}
	if cfg.LangSmithProject != "smart-food-manager" || !cfg.LangSmithCaptureContent {
		t.Errorf("unexpected LangSmith defaults: %+v", cfg)
	}
	// No key means no OpenAI agent, so the model is left for the agent to
	// default rather than guessed here.
	if cfg.OpenAIAPIKey != "" || cfg.OpenAIModel != "" || cfg.OpenAITimeout != 60*time.Second {
		t.Errorf("unexpected OpenAI defaults: %+v", cfg)
	}
	// Same for Mistral: no key, and the client owns the model default.
	if cfg.MistralAPIKey != "" || cfg.MistralModel != "" || cfg.MistralTimeout != 60*time.Second {
		t.Errorf("unexpected Mistral defaults: %+v", cfg)
	}
}

func TestLangSmithTracingSwitch(t *testing.T) {
	// LANGSMITH_TRACING is LangSmith's own switch; an existing .env uses it to
	// stop exporting without deleting the key.
	t.Setenv("LANGSMITH_API_KEY", "ls-key")
	t.Setenv("LANGSMITH_TRACING", "false")
	cfg := Load()
	if cfg.LLMTracingEnabled() || cfg.LangSmithExportKey() != "" {
		t.Errorf("tracing still on with the switch off: %+v", cfg)
	}

	t.Setenv("LANGSMITH_TRACING", "true")
	if cfg := Load(); !cfg.LLMTracingEnabled() || cfg.LangSmithExportKey() != "ls-key" {
		t.Errorf("tracing not enabled with key and switch: %+v", cfg)
	}

	// Absent switch, a key alone is enough.
	t.Setenv("LANGSMITH_TRACING", "")
	if !Load().LLMTracingEnabled() {
		t.Error("tracing off when LANGSMITH_TRACING is unset")
	}
}

func TestLangSmithContentCaptureRequiresTracing(t *testing.T) {
	// Prompt text rides on spans that also reach the OTel collector, so it is
	// only ever captured when someone opted into LLM tracing.
	t.Setenv("LANGSMITH_CAPTURE_CONTENT", "true")
	t.Setenv("LANGSMITH_API_KEY", "")
	if Load().CaptureLLMContent() {
		t.Error("content captured with LangSmith disabled")
	}

	t.Setenv("LANGSMITH_API_KEY", "ls-key")
	if !Load().CaptureLLMContent() {
		t.Error("content not captured with LangSmith enabled")
	}

	t.Setenv("LANGSMITH_CAPTURE_CONTENT", "false")
	cfg := Load()
	if !cfg.LLMTracingEnabled() || cfg.CaptureLLMContent() {
		t.Errorf("opt-out ignored: %+v", cfg)
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
	t.Setenv("GEMINI_ALT_MODEL", "alt-model")
	t.Setenv("LANGSMITH_API_KEY", "ls-key")
	t.Setenv("LANGSMITH_PROJECT", "food-experiments")
	t.Setenv("LANGSMITH_ENDPOINT", "langsmith.internal")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-5.4-mini")
	t.Setenv("OPENAI_TIMEOUT_SECONDS", "12")
	t.Setenv("MISTRAL_API_KEY", "mi-test")
	t.Setenv("MISTRAL_MODEL", "mistral-small-latest")

	cfg := Load()
	if cfg.Port != "9090" || cfg.DatabaseURL != "postgres://test" || cfg.JWTSecret != "configured-secret" {
		t.Errorf("core overrides not loaded: %+v", cfg)
	}
	if cfg.JWTExpiry != 15*time.Minute || cfg.RateLimitRPS != 2.5 || cfg.RateLimitBurst != 7 {
		t.Errorf("numeric overrides not loaded: %+v", cfg)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "https://app.example.com" ||
		cfg.ServiceName != "food-api" || cfg.Environment != "test" {
		t.Errorf("service overrides not loaded: %+v", cfg)
	}
	if cfg.GeminiAPIKey != "key" || cfg.GeminiBaseURL != "https://gemini.example.com" ||
		cfg.GeminiModel != "test-model" || cfg.GeminiTimeout != 9*time.Second ||
		cfg.GeminiAltModel != "alt-model" {
		t.Errorf("Gemini overrides not loaded: %+v", cfg)
	}
	if !cfg.LLMTracingEnabled() || cfg.LangSmithProject != "food-experiments" ||
		cfg.LangSmithEndpoint != "langsmith.internal" {
		t.Errorf("LangSmith overrides not loaded: %+v", cfg)
	}
	if cfg.OpenAIAPIKey != "sk-test" || cfg.OpenAIModel != "gpt-5.4-mini" ||
		cfg.OpenAITimeout != 12*time.Second {
		t.Errorf("OpenAI overrides not loaded: %+v", cfg)
	}
	if cfg.MistralAPIKey != "mi-test" || cfg.MistralModel != "mistral-small-latest" {
		t.Errorf("Mistral overrides not loaded: %+v", cfg)
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

func TestAllowedOriginsAcceptsAList(t *testing.T) {
	// Cloudflare Pages gives every branch its own origin, so production and
	// previews have to coexist in one variable.
	t.Setenv("ALLOWED_ORIGIN", "https://app.example.com, https://preview.pages.dev/ ,, ")

	got := Load().AllowedOrigins
	want := []string{"https://app.example.com", "https://preview.pages.dev"}
	if len(got) != len(want) {
		t.Fatalf("AllowedOrigins = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllowedOrigins = %q, want %q (spaces trimmed, trailing slash dropped)", got, want)
		}
	}
}
