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
