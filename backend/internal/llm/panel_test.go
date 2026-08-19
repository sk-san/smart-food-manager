package llm

import (
	"errors"
	"testing"

	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

func TestNewPanelNeedsAProvider(t *testing.T) {
	// Without a key there is nothing to fan out to. Returning an error lets
	// the server leave the route unregistered instead of serving one that can
	// only fail.
	_, err := NewPanel(config.Config{}, tracing.NewRecorder(nil, false), "panel", "system")
	if !errors.Is(err, ErrNoProviders) {
		t.Errorf("err = %v, want ErrNoProviders", err)
	}
}

func TestNewPanelRosterFollowsConfiguredProviders(t *testing.T) {
	rec := tracing.NewRecorder(nil, false)

	// One provider still forms a panel, because a second Gemini model gives
	// two comparable drafts from one key.
	p, err := NewPanel(config.Config{
		GeminiAPIKey:   "k",
		GeminiModel:    "gemini-2.5-flash",
		GeminiAltModel: "gemini-3.6-flash",
	}, rec, "panel", "system")
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	if got := p.Describe(); len(got) != 2 {
		t.Errorf("roster = %+v, want two Gemini models", got)
	}

	// An alt model equal to the primary would be the same model twice.
	p, err = NewPanel(config.Config{
		GeminiAPIKey:   "k",
		GeminiModel:    "gemini-2.5-flash",
		GeminiAltModel: "gemini-2.5-flash",
	}, rec, "panel", "system")
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	if got := p.Describe(); len(got) != 1 {
		t.Errorf("roster = %+v, want the duplicate dropped", got)
	}

	// A second provider joins when its key is set.
	p, err = NewPanel(config.Config{
		GeminiAPIKey:  "k",
		GeminiModel:   "gemini-2.5-flash",
		MistralAPIKey: "m",
	}, rec, "panel", "system")
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	roster := p.Describe()
	if len(roster) != 2 {
		t.Fatalf("roster = %+v, want Gemini and Mistral", roster)
	}
	providers := map[string]bool{}
	for _, d := range roster {
		providers[d.Provider] = true
	}
	if !providers["gcp.gemini"] || !providers["mistral"] {
		t.Errorf("providers = %v, want both", providers)
	}
}

func TestNewPanelWorksWithoutGemini(t *testing.T) {
	// Mistral alone must still merge its own draft, or the endpoint would
	// return a list of drafts and no answer.
	p, err := NewPanel(config.Config{MistralAPIKey: "m"}, tracing.NewRecorder(nil, false), "panel", "system")
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	if got := p.Describe(); len(got) != 1 || got[0].Provider != "mistral" {
		t.Errorf("roster = %+v", got)
	}
}
