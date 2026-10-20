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

// deleteRecordingInfraManager records DeleteOrgDeploymentPipeline calls; every other method panics.
type deleteRecordingInfraManager struct {
	services.InfraResourceManager
	deletes int
}

func (m *deleteRecordingInfraManager) DeleteOrgDeploymentPipeline(context.Context, string, string) error {
	m.deletes++
	return nil
}

func setDeploymentPipelineManagedMode(t *testing.T, enabled bool) {
	t.Helper()
	orig := config.GetConfig().EnableDeploymentPipelineManagedMode
	config.GetConfig().EnableDeploymentPipelineManagedMode = enabled
	t.Cleanup(func() { config.GetConfig().EnableDeploymentPipelineManagedMode = orig })
}

func TestDeploymentPipelineManagedModeRefusesPipelineChanges(t *testing.T) {
	setDeploymentPipelineManagedMode(t, true)
	// A nil manager: reaching it would panic, so a 403 proves the guard ran first.
	ctrl := &infraResourceController{}

	for _, tc := range []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"create", http.MethodPost, ctrl.CreateOrgDeploymentPipeline},
		{"update", http.MethodPut, ctrl.UpdateOrgDeploymentPipeline},
		{"delete", http.MethodDelete, ctrl.DeleteOrgDeploymentPipeline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/orgs/acme/deployment-pipelines/default", strings.NewReader(`{}`))
			rr := httptest.NewRecorder()

			tc.handler(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
			}
			if !strings.Contains(rr.Body.String(), "managed outside Agent Manager") {
				t.Errorf("body = %q, want it to say pipelines are managed outside Agent Manager", rr.Body.String())
			}
		})
	}
}

func TestDeploymentPipelineManagedModeOffLetsPipelineChangesThrough(t *testing.T) {
	setDeploymentPipelineManagedMode(t, false)
	manager := &deleteRecordingInfraManager{}
	ctrl := &infraResourceController{infraResourceManager: manager}

	req := httptest.NewRequest(http.MethodDelete, "/orgs/acme/deployment-pipelines/default", nil)
	req.SetPathValue("pipelineName", "default")
	rr := httptest.NewRecorder()

	ctrl.DeleteOrgDeploymentPipeline(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if manager.deletes != 1 {
		t.Errorf("DeleteOrgDeploymentPipeline calls = %d, want 1", manager.deletes)
	}
}
