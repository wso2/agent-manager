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

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// createOnlyAgentService satisfies AgentManagerService for the CreateAgent handler,
// leaving every other method nil so an unexpected call panics.
type createOnlyAgentService struct {
	services.AgentManagerService
}

func (createOnlyAgentService) CreateAgent(
	_ context.Context, _ string, _ string, _ *spec.CreateAgentRequest,
) error {
	return nil
}

// TestCreateAgent_DoesNotEchoSecretValues pins the 202 body to the contract's
// promise that a value is "omitted for secrets in responses". The handler used to
// copy the decoded payload straight back out, so every submitted secret — and, for
// kind-based agents, the kind's stored secret defaults — crossed the wire in
// plaintext.
func TestCreateAgent_DoesNotEchoSecretValues(t *testing.T) {
	body, err := json.Marshal(spec.CreateAgentRequest{
		Name:        "yolo",
		DisplayName: "Yolo",
		Provisioning: spec.Provisioning{
			Type:      "internal",
			AgentKind: &spec.ProvisioningAgentKind{Name: "chatbot", Version: "v1"},
		},
		Configurations: &spec.Configurations{
			Env: []spec.EnvironmentVariable{
				{Key: "OPENAI_API_KEY", Value: spec.PtrString("sk-super-secret"), IsSensitive: spec.PtrBool(true)},
				{Key: "LOG_LEVEL", Value: spec.PtrString("debug")},
			},
			Files: []spec.FileMount{
				{
					Key:         "creds.json",
					MountPath:   "/etc/creds.json",
					Value:       spec.PtrString("file-super-secret"),
					IsSensitive: spec.PtrBool(true),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	ctrl := NewAgentController(createOnlyAgentService{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/orgs/o/projects/default/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ctrl.CreateAgent(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-super-secret") {
		t.Errorf("202 body echoes the submitted env secret; body=%s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "file-super-secret") {
		t.Errorf("202 body echoes the submitted file-mount secret; body=%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "debug") {
		t.Errorf("202 body dropped a non-sensitive value; body=%s", w.Body.String())
	}
}
