package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config")

	expiry := time.Date(2026, 5, 15, 12, 30, 45, 0, time.UTC)
	in := Config{
		CurrentInstance: "dev",
		Instances: map[string]Instance{
			"dev": {
				URL:      "https://am.example.com",
				TokenURL: "https://idp.example.com/oauth2/token",
				Auth: AuthConfig{
					ClientID:     "cid",
					ClientSecret: "csecret",
					AccessToken:  "atok",
					RefreshToken: "rtok",
					ExpiresAt:    expiry,
				},
			},
		},
	}

	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Errorf("file mode = %o, want 0600", mode)
		}
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got, ok := out.Instances["dev"]
	if !ok {
		t.Fatalf("instance dev missing from loaded config")
	}
	want := in.Instances["dev"]
	if got.URL != want.URL || got.TokenURL != want.TokenURL {
		t.Errorf("instance URL/TokenURL mismatch: got %+v want %+v", got, want)
	}
	if got.Auth.AccessToken != want.Auth.AccessToken ||
		got.Auth.RefreshToken != want.Auth.RefreshToken ||
		got.Auth.ClientID != want.Auth.ClientID ||
		got.Auth.ClientSecret != want.Auth.ClientSecret {
		t.Errorf("auth fields mismatch: got %+v want %+v", got.Auth, want.Auth)
	}
	if !got.Auth.ExpiresAt.Equal(expiry) {
		t.Errorf("ExpiresAt = %v, want %v", got.Auth.ExpiresAt, expiry)
	}
	if out.CurrentInstance != "dev" {
		t.Errorf("CurrentInstance = %q, want %q", out.CurrentInstance, "dev")
	}
	if out.Path != path {
		t.Errorf("Path = %q, want %q", out.Path, path)
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config")
	}
	if cfg.Path != path {
		t.Errorf("Path = %q, want %q", cfg.Path, path)
	}
	if len(cfg.Instances) != 0 {
		t.Errorf("expected no instances, got %d", len(cfg.Instances))
	}
}
