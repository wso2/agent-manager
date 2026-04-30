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

package org

import (
	"context"
	"net/http"
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/config"
)

func TestUse_ValidOrg(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com", CurrentOrg: "old-org"},
		},
	})
	clientFn, closeFn := newTestClient(t, http.StatusOK, amsvc.OrganizationResponse{
		Name:        "acme",
		DisplayName: "Acme Corp",
		Namespace:   "acme",
	})
	defer closeFn()

	err := runUse(context.Background(), &UseOptions{IO: io, Config: cfgFn, Client: clientFn, Name: "acme"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["org"] != "acme" {
		t.Errorf("org = %v, want acme", data["org"])
	}
	if env["org"] != "acme" {
		t.Errorf("scope org = %v, want acme", env["org"])
	}

	cfg, _ := cfgFn()
	if cfg.Instances["prod"].CurrentOrg != "acme" {
		t.Errorf("persisted current_org = %q, want acme", cfg.Instances["prod"].CurrentOrg)
	}
}

func TestUse_OrgNotFound(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com", CurrentOrg: "old-org"},
		},
	})
	reason := "not found"
	clientFn, closeFn := newTestClient(t, http.StatusNotFound, amsvc.ErrorResponse{
		Code:    "ORG_NOT_FOUND",
		Message: "Organization 'nope' not found",
		Reason:  &reason,
	})
	defer closeFn()

	err := runUse(context.Background(), &UseOptions{IO: io, Config: cfgFn, Client: clientFn, Name: "nope"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "ORG_NOT_FOUND" {
		t.Errorf("code = %v, want ORG_NOT_FOUND", errBody["code"])
	}

	cfg, _ := cfgFn()
	if cfg.Instances["prod"].CurrentOrg != "old-org" {
		t.Errorf("current_org should not change on failure, got %q", cfg.Instances["prod"].CurrentOrg)
	}
}

func TestUse_NoInstance(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{})
	clientFn, closeFn := newTestClient(t, http.StatusOK, nil)
	defer closeFn()

	err := runUse(context.Background(), &UseOptions{IO: io, Config: cfgFn, Client: clientFn, Name: "acme"})
	if err == nil {
		t.Fatal("expected error when no instance configured")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.NoInstance {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.NoInstance)
	}
}

func TestUse_EmptyName(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})
	clientFn, closeFn := newTestClient(t, http.StatusOK, nil)
	defer closeFn()

	err := runUse(context.Background(), &UseOptions{IO: io, Config: cfgFn, Client: clientFn, Name: ""})
	if err == nil {
		t.Fatal("expected error for empty org name")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}
