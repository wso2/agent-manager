package instance

import (
	"errors"
	"testing"

	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
)

type fakePrompter struct {
	confirmDeletionErr error
	confirmDeletionArg string
	calls              int
}

func (p *fakePrompter) ConfirmDeletion(required string) error {
	p.calls++
	p.confirmDeletionArg = required
	return p.confirmDeletionErr
}

func TestRemove_Success(t *testing.T) {
	io, out := newTestIO()
	io.SetTerminal(true, true, true)
	prompter := &fakePrompter{}
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod":    {URL: "https://prod.example.com"},
			"staging": {URL: "https://staging.example.com"},
		},
	})

	err := runRemove(&RemoveOptions{IO: io, Prompter: prompter, Config: cfgFn, Name: "staging", Yes: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 1 {
		t.Errorf("prompter calls = %d, want 1", prompter.calls)
	}
	if prompter.confirmDeletionArg != "staging" {
		t.Errorf("confirmation arg = %q, want staging", prompter.confirmDeletionArg)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["instance"] != "staging" || data["removed"] != true {
		t.Errorf("data = %v, want {instance=staging, removed=true}", data)
	}

	cfg, _ := cfgFn()
	if _, ok := cfg.Instances["staging"]; ok {
		t.Error("staging should have been removed from config")
	}
	if cfg.CurrentInstance != "prod" {
		t.Errorf("current_instance = %q, want prod (should not change)", cfg.CurrentInstance)
	}
}

func TestRemove_CurrentInstanceClearsSelection(t *testing.T) {
	io, _ := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})

	err := runRemove(&RemoveOptions{IO: io, Prompter: &fakePrompter{}, Config: cfgFn, Name: "prod", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg, _ := cfgFn()
	if cfg.CurrentInstance != "" {
		t.Errorf("current_instance = %q, want empty after removing current", cfg.CurrentInstance)
	}
}

func TestRemove_UnknownInstance(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})

	err := runRemove(&RemoveOptions{IO: io, Prompter: &fakePrompter{}, Config: cfgFn, Name: "nope", Yes: true})
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.NoInstance {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.NoInstance)
	}
}

func TestRemove_NonTTYWithoutYes(t *testing.T) {
	io, _, _, _ := iostreams.Test()
	io.SetTerminal(false, false, false)
	io.JSON = true
	cfgFn := writeConfig(t, &config.Config{
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})

	err := runRemove(&RemoveOptions{IO: io, Prompter: &fakePrompter{}, Config: cfgFn, Name: "prod", Yes: false})
	if err == nil {
		t.Fatal("expected error for non-TTY without --yes")
	}
}

func TestRemove_YesSkipsPrompt(t *testing.T) {
	io, _ := newTestIO()
	prompter := &fakePrompter{}
	cfgFn := writeConfig(t, &config.Config{
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})

	err := runRemove(&RemoveOptions{IO: io, Prompter: prompter, Config: cfgFn, Name: "prod", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 0 {
		t.Errorf("prompter calls = %d, want 0 with --yes", prompter.calls)
	}
}

func TestRemove_ConfirmationMismatch(t *testing.T) {
	io, out := newTestIO()
	io.SetTerminal(true, true, true)
	prompter := &fakePrompter{confirmDeletionErr: errors.New("confirmation mismatch")}
	cfgFn := writeConfig(t, &config.Config{
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})

	err := runRemove(&RemoveOptions{IO: io, Prompter: prompter, Config: cfgFn, Name: "prod", Yes: false})
	if err == nil {
		t.Fatal("expected error from confirmation mismatch")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}
