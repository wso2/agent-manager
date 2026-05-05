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
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func newTestIO() (*iostreams.IOStreams, *bytes.Buffer) {
	io, _, out, _ := iostreams.Test()
	io.JSON = true
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

func TestList_TextOutput(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = false
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
	got := out.String()
	if !strings.Contains(got, "prod") {
		t.Errorf("output should contain instance name, got %q", got)
	}
	if !strings.Contains(got, "https://prod.example.com") {
		t.Errorf("output should contain URL, got %q", got)
	}
	if !strings.Contains(got, "staging") {
		t.Errorf("output should contain second instance, got %q", got)
	}
}

func TestList_TSVOmitsMarker(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = false
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com", CurrentOrg: "acme"},
		},
	})

	err := runList(&ListOptions{IO: io, Config: cfgFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "*") {
		t.Errorf("TSV output should not contain * marker, got %q", got)
	}
}
