// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

func TestSaveConcurrentDoesNotCollide(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	const writers = 16
	const rounds = 25

	var wg sync.WaitGroup
	errs := make(chan error, writers*rounds)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				cfg := Config{
					CurrentInstance: "dev",
					Instances: map[string]Instance{
						"dev": {URL: fmt.Sprintf("https://w%d-r%d.example.com", id, r)},
					},
				}
				if err := Save(path, cfg); err != nil {
					errs <- fmt.Errorf("writer %d round %d: %w", id, r, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Save error: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load after concurrent saves: %v", err)
	}
	if _, ok := out.Instances["dev"]; !ok {
		t.Fatalf("expected dev instance after concurrent saves, got %+v", out.Instances)
	}

	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("expected no leftover temp files, got %v", leftovers)
	}
}

func TestLinkedProjectRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	cfg := Config{
		CurrentInstance: "dev",
		Instances:       map[string]Instance{"dev": {URL: "https://am.example.com"}},
	}
	cfg.LinkProject("/home/user/my-app", LinkedProject{
		Org:         "acme",
		Project:     "chatbot",
		Environment: "dev-env",
		Agent:       "assistant",
	})
	cfg.LinkProject("/home/user/other", LinkedProject{
		Org:     "acme",
		Project: "other-proj",
	})

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	lp, ok := loaded.LinkedProjects["/home/user/my-app"]
	if !ok {
		t.Fatal("expected linked project for /home/user/my-app")
	}
	if lp.Org != "acme" || lp.Project != "chatbot" || lp.Environment != "dev-env" || lp.Agent != "assistant" {
		t.Errorf("linked project mismatch: %+v", lp)
	}
	if _, ok := loaded.LinkedProjects["/home/user/other"]; !ok {
		t.Fatal("expected linked project for /home/user/other")
	}
}

func TestGetLinkedProjectAncestorWalk(t *testing.T) {
	cfg := Config{
		LinkedProjects: map[string]LinkedProject{
			"/home/user/projects/repo": {
				Org:     "acme",
				Project: "myproj",
			},
		},
	}

	tests := []struct {
		startDir string
		wantOrg  string
		wantNil  bool
	}{
		{"/home/user/projects/repo", "acme", false},
		{"/home/user/projects/repo/src/cmd", "acme", false},
		{"/home/user/projects/repo/deep/nested/dir", "acme", false},
		{"/home/user/projects/other", "", true},
		{"/home/user", "", true},
		{"/", "", true},
	}

	for _, tt := range tests {
		gotDir, got := cfg.GetLinkedProject(tt.startDir)
		if tt.wantNil {
			if got != nil {
				t.Errorf("GetLinkedProject(%q) = (%q, %+v), want nil", tt.startDir, gotDir, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("GetLinkedProject(%q) = nil, want org=%q", tt.startDir, tt.wantOrg)
			continue
		}
		if got.Org != tt.wantOrg {
			t.Errorf("GetLinkedProject(%q).Org = %q, want %q", tt.startDir, got.Org, tt.wantOrg)
		}
	}
}

func TestGetLinkedProjectReturnsDir(t *testing.T) {
	cfg := Config{
		LinkedProjects: map[string]LinkedProject{
			"/home/user/app": {Org: "acme", Project: "app"},
		},
	}

	if dir, _ := cfg.GetLinkedProject("/home/user/app/src"); dir != "/home/user/app" {
		t.Errorf("dir from subdir = %q, want /home/user/app", dir)
	}
	if dir, _ := cfg.GetLinkedProject("/home/user/other"); dir != "" {
		t.Errorf("dir for unlinked = %q, want empty", dir)
	}
}

func TestUnlinkProject(t *testing.T) {
	cfg := Config{
		LinkedProjects: map[string]LinkedProject{
			"/home/user/app": {Org: "acme", Project: "app"},
		},
	}

	cfg.UnlinkProject("/home/user/app")
	if len(cfg.LinkedProjects) != 0 {
		t.Errorf("expected empty linked projects after unlink, got %d", len(cfg.LinkedProjects))
	}

	if _, lp := cfg.GetLinkedProject("/home/user/app"); lp != nil {
		t.Errorf("expected nil after unlink, got %+v", lp)
	}
}

func TestGetLinkedProjectClosestMatch(t *testing.T) {
	cfg := Config{
		LinkedProjects: map[string]LinkedProject{
			"/home/user/projects":      {Org: "parent-org", Project: "parent"},
			"/home/user/projects/repo": {Org: "child-org", Project: "child"},
		},
	}

	_, got := cfg.GetLinkedProject("/home/user/projects/repo/src")
	if got == nil {
		t.Fatal("expected linked project, got nil")
	}
	if got.Org != "child-org" {
		t.Errorf("expected closest match child-org, got %q", got.Org)
	}

	_, got = cfg.GetLinkedProject("/home/user/projects/other")
	if got == nil {
		t.Fatal("expected linked project, got nil")
	}
	if got.Org != "parent-org" {
		t.Errorf("expected parent-org fallback, got %q", got.Org)
	}
}

func TestClearLinksIfSwitching(t *testing.T) {
	cfg := &Config{
		CurrentInstance: "prod",
		LinkedProjects:  map[string]LinkedProject{"/a": {Org: "o", Project: "p"}},
	}
	if n := cfg.ClearLinksIfSwitching("prod"); n != 0 {
		t.Errorf("same instance: cleared = %d, want 0", n)
	}
	if len(cfg.LinkedProjects) != 1 {
		t.Errorf("same instance: links len = %d, want 1", len(cfg.LinkedProjects))
	}
	if n := cfg.ClearLinksIfSwitching("staging"); n != 1 {
		t.Errorf("different instance: cleared = %d, want 1", n)
	}
	if len(cfg.LinkedProjects) != 0 {
		t.Errorf("different instance: links len = %d, want 0", len(cfg.LinkedProjects))
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

func TestClearLinkedProjects(t *testing.T) {
	cfg := &Config{
		LinkedProjects: map[string]LinkedProject{
			"/a": {Org: "o1", Project: "p1"},
			"/b": {Org: "o2", Project: "p2"},
		},
	}

	n := cfg.ClearLinkedProjects()
	if n != 2 {
		t.Errorf("returned count = %d, want 2", n)
	}
	if len(cfg.LinkedProjects) != 0 {
		t.Errorf("LinkedProjects len = %d, want 0", len(cfg.LinkedProjects))
	}
}

func TestClearLinkedProjectsEmpty(t *testing.T) {
	cfg := &Config{}
	n := cfg.ClearLinkedProjects()
	if n != 0 {
		t.Errorf("returned count = %d, want 0", n)
	}
}
