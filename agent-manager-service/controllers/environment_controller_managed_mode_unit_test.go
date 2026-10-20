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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// deleteRecordingEnvService records DeleteEnvironment calls; every other method panics.
type deleteRecordingEnvService struct {
	services.EnvironmentService
	deletes int
}

func (s *deleteRecordingEnvService) DeleteEnvironment(context.Context, string, string) error {
	s.deletes++
	return nil
}

func setEnvironmentManagedMode(t *testing.T, enabled bool) {
	t.Helper()
	orig := config.GetConfig().EnableEnvironmentManagedMode
	config.GetConfig().EnableEnvironmentManagedMode = enabled
	t.Cleanup(func() { config.GetConfig().EnableEnvironmentManagedMode = orig })
}

func TestEnvironmentManagedModeRefusesEnvironmentChanges(t *testing.T) {
	setEnvironmentManagedMode(t, true)
	// A nil service: reaching it would panic, so a 403 proves the guard ran first.
	ctrl := &environmentController{}

	for _, tc := range []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"create", http.MethodPost, ctrl.CreateEnvironment},
		{"update", http.MethodPut, ctrl.UpdateEnvironment},
		{"delete", http.MethodDelete, ctrl.DeleteEnvironment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/orgs/acme/environments/dev", strings.NewReader(`{}`))
			rr := httptest.NewRecorder()

			tc.handler(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
			}
			if !strings.Contains(rr.Body.String(), "managed outside Agent Manager") {
				t.Errorf("body = %q, want it to say environments are managed outside Agent Manager", rr.Body.String())
			}
		})
	}
}

func TestEnvironmentManagedModeOffLetsEnvironmentChangesThrough(t *testing.T) {
	setEnvironmentManagedMode(t, false)
	svc := &deleteRecordingEnvService{}
	ctrl := &environmentController{environmentService: svc}

	req := httptest.NewRequest(http.MethodDelete, "/orgs/acme/environments/dev", nil)
	req.SetPathValue("envID", "dev")
	rr := httptest.NewRecorder()

	ctrl.DeleteEnvironment(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if svc.deletes != 1 {
		t.Errorf("DeleteEnvironment calls = %d, want 1", svc.deletes)
	}
}
