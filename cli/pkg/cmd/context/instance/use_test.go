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

package instance

import (
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

func TestUse_SwitchesInstance(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "staging",
		Instances: map[string]config.Instance{
			"prod":    {URL: "https://prod.example.com"},
			"staging": {URL: "https://staging.example.com"},
		},
	})

	err := runUse(&UseOptions{IO: io, Config: cfgFn, Name: "prod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["instance"] != "prod" {
		t.Errorf("instance = %v, want prod", data["instance"])
	}

	// Verify persisted
	cfg, _ := cfgFn()
	if cfg.CurrentInstance != "prod" {
		t.Errorf("persisted current_instance = %q, want prod", cfg.CurrentInstance)
	}
}

func TestUse_UnknownInstance(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "staging",
		Instances: map[string]config.Instance{
			"staging": {URL: "https://staging.example.com"},
		},
	})

	err := runUse(&UseOptions{IO: io, Config: cfgFn, Name: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.NoInstance {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.NoInstance)
	}
}

func TestUse_DoesNotTouchCurrentOrg(t *testing.T) {
	io, _ := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "staging",
		Instances: map[string]config.Instance{
			"prod":    {URL: "https://prod.example.com", CurrentOrg: "prod-org"},
			"staging": {URL: "https://staging.example.com", CurrentOrg: "staging-org"},
		},
	})

	err := runUse(&UseOptions{IO: io, Config: cfgFn, Name: "prod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, _ := cfgFn()
	if cfg.Instances["prod"].CurrentOrg != "prod-org" {
		t.Errorf("prod current_org = %q, want prod-org", cfg.Instances["prod"].CurrentOrg)
	}
	if cfg.Instances["staging"].CurrentOrg != "staging-org" {
		t.Errorf("staging current_org = %q, want staging-org", cfg.Instances["staging"].CurrentOrg)
	}
}

func TestUse_ClearsLinkedProjects(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "staging",
		Instances: map[string]config.Instance{
			"prod":    {URL: "https://prod.example.com"},
			"staging": {URL: "https://staging.example.com"},
		},
		LinkedProjects: map[string]config.LinkedProject{
			"/path/a": {Org: "o1", Project: "p1"},
			"/path/b": {Org: "o2", Project: "p2"},
		},
	})

	err := runUse(&UseOptions{IO: io, Config: cfgFn, Name: "prod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, _ := cfgFn()
	if len(cfg.LinkedProjects) != 0 {
		t.Errorf("LinkedProjects len after switch = %d, want 0", len(cfg.LinkedProjects))
	}

	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	cleared, ok := data["cleared_links"]
	if !ok {
		t.Fatal("expected cleared_links field in JSON response")
	}
	if cleared.(float64) != 2 {
		t.Errorf("cleared_links = %v, want 2", cleared)
	}
}

func TestUse_SameInstanceKeepsLinks(t *testing.T) {
	io, _ := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
		LinkedProjects: map[string]config.LinkedProject{
			"/path/a": {Org: "o1", Project: "p1"},
		},
	})

	err := runUse(&UseOptions{IO: io, Config: cfgFn, Name: "prod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, _ := cfgFn()
	if len(cfg.LinkedProjects) != 1 {
		t.Errorf("LinkedProjects len = %d, want 1 (no-op switch should preserve)", len(cfg.LinkedProjects))
	}
}
