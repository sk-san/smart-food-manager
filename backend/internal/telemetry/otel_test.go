package telemetry

import "testing"

func TestNormalizeOTLPEndpoint(t *testing.T) {
	// LangSmith documents regional endpoints as URLs, but the OTLP HTTP
	// exporter takes a bare host and appends its own path.
	tests := map[string]string{
		"https://eu.api.smith.langchain.com":             "eu.api.smith.langchain.com",
		"https://api.smith.langchain.com/otel/v1/traces": "api.smith.langchain.com",
		"api.smith.langchain.com":                        "api.smith.langchain.com",
		"  https://eu.api.smith.langchain.com/  ":        "eu.api.smith.langchain.com",
		"langsmith.internal:4318":                        "langsmith.internal:4318",
		"":                                               "",
	}
	for in, want := range tests {
		if got := normalizeOTLPEndpoint(in); got != want {
			t.Errorf("normalizeOTLPEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCollectorConfigured(t *testing.T) {
	// Every OTLP endpoint variable is cleared first: the SDK would otherwise
	// default to localhost:4317, which is exactly the silent fallback this
	// check exists to avoid in a deployed service.
	for _, k := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		t.Setenv(k, "")
	}
	if collectorConfigured() {
		t.Error("reported a collector with every endpoint unset")
	}

	// A signal-specific endpoint counts: a deployment may export only traces.
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4317")
	if !collectorConfigured() {
		t.Error("did not report a collector when the traces endpoint is set")
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	if !collectorConfigured() {
		t.Error("did not report a collector when the general endpoint is set")
	}
}
