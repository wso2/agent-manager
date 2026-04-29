package instance

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
)

func newTestIO() (*iostreams.IOStreams, *bytes.Buffer) {
	io, _, out, _ := iostreams.Test()
	return io, out
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}

func writeConfig(t *testing.T, cfg *config.Config) func() (*config.Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	cfg.Path = path
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return func() (*config.Config, error) { return config.Load(path) }
}

func TestList_Empty(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{})

	err := runList(&ListOptions{IO: io, Config: cfgFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["current"] != "" {
		t.Errorf("current = %v, want empty", data["current"])
	}
	instances := data["instances"].([]any)
	if len(instances) != 0 {
		t.Errorf("instances = %v, want empty", instances)
	}
}

func TestList_MultipleInstances(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod":    {URL: "https://prod.example.com", CurrentOrg: "acme"},
			"staging": {URL: "https://staging.example.com"},
		},
	})

	err := runList(&ListOptions{IO: io, Config: cfgFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["current"] != "prod" {
		t.Errorf("current = %v, want prod", data["current"])
	}
	instances := data["instances"].([]any)
	if len(instances) != 2 {
		t.Errorf("len(instances) = %d, want 2", len(instances))
	}
}
