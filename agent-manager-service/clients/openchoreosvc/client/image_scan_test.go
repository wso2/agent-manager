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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// An agent whose build settings already keep the scan from failing the build, at HIGH.
const componentWithImageScanJSON = `{
  "metadata": {"name": "support-bot"},
  "spec": {
    "componentType": {"name": "proxy/agent-api"},
    "owner": {"projectName": "proj"},
    "workflow": {"name": "amp-ballerina-buildpack", "parameters": {
      "endpoints": [],
      "imageScan": {"failBuild": false, "severity": "HIGH"}
    }}
  }
}`

// savedImageScanParameters runs UpdateComponentBuildParameters against a fake OpenChoreo
// holding componentWithImageScanJSON and returns the build settings it writes back.
func savedImageScanParameters(t *testing.T, req UpdateComponentBuildParametersRequest) map[string]any {
	t.Helper()
	var putWorkflowParameters map[string]any
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			var sent struct {
				Spec struct {
					Workflow struct {
						Parameters map[string]any `json:"parameters"`
					} `json:"workflow"`
				} `json:"spec"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			putWorkflowParameters = sent.Spec.Workflow.Parameters
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(componentWithImageScanJSON))
		require.NoError(t, err)
	}))
	req.AgentType = AgentTypeConfig{Type: "agent-api", SubType: "chat-api"}
	require.NoError(t, client.UpdateComponentBuildParameters(context.Background(), "acme", "proj", "support-bot", req))
	require.NotNil(t, putWorkflowParameters, "expected the update to PUT the component back")
	return putWorkflowParameters
}

// An update without the setting keeps the saved one, so a caller that doesn't know about
// it can't turn the scan back to failing the build.
func TestUpdateComponentBuildParameters_KeepsImageScanWhenNotSent(t *testing.T) {
	saved := savedImageScanParameters(t, UpdateComponentBuildParametersRequest{})
	assert.Equal(t, map[string]any{"failBuild": false, "severity": "HIGH"}, saved["imageScan"])
}

// A sent setting replaces the saved one entirely: the saved severity is not merged in.
func TestUpdateComponentBuildParameters_ReplacesImageScanWhenSent(t *testing.T) {
	failBuild := true
	saved := savedImageScanParameters(t, UpdateComponentBuildParametersRequest{
		ImageScan: &ImageScanConfig{FailBuild: &failBuild},
	})
	assert.Equal(t, map[string]any{"failBuild": true}, saved["imageScan"])
}

// An empty setting removes the saved one, so the next build uses the workflow's defaults.
func TestUpdateComponentBuildParameters_EmptyImageScanRemovesSavedOne(t *testing.T) {
	saved := savedImageScanParameters(t, UpdateComponentBuildParametersRequest{ImageScan: &ImageScanConfig{}})
	assert.NotContains(t, saved, "imageScan")
}

// Each field is saved under the name the build workflow reads (imageScan.failBuild and
// imageScan.severity); a misspelt name would be saved and silently ignored by the build.
func TestSetImageScanWorkflowParameter_SavesEveryFieldUnderTheWorkflowName(t *testing.T) {
	failBuild, severity := false, "MEDIUM"
	params := map[string]any{}

	setImageScanWorkflowParameter(params, &ImageScanConfig{FailBuild: &failBuild, Severity: &severity})

	assert.Equal(t, map[string]any{"failBuild": false, "severity": "MEDIUM"}, params["imageScan"])
}

// Reading the agent returns the saved setting, so a caller can show it.
func TestConvertComponentFromTyped_ReturnsSavedImageScan(t *testing.T) {
	var component gen.Component
	require.NoError(t, json.Unmarshal([]byte(componentWithImageScanJSON), &component))

	agent, err := convertComponentFromTyped(&component)

	require.NoError(t, err)
	require.NotNil(t, agent.ImageScan)
	require.NotNil(t, agent.ImageScan.FailBuild)
	assert.False(t, *agent.ImageScan.FailBuild)
	require.NotNil(t, agent.ImageScan.Severity)
	assert.Equal(t, "HIGH", *agent.ImageScan.Severity)
}

// No saved setting reads as none, and one that can't be read is left out rather than
// failing, because callers drop an agent that fails to convert from the agent list.
func TestConvertComponentFromTyped_MissingOrUnreadableImageScanIsLeftOut(t *testing.T) {
	for name, value := range map[string]any{"missing": nil, "unreadable": "not an object"} {
		t.Run(name, func(t *testing.T) {
			var component gen.Component
			require.NoError(t, json.Unmarshal([]byte(componentWithImageScanJSON), &component))
			if value == nil {
				delete(*component.Spec.Workflow.Parameters, "imageScan")
			} else {
				(*component.Spec.Workflow.Parameters)["imageScan"] = value
			}

			agent, err := convertComponentFromTyped(&component)

			require.NoError(t, err)
			assert.Nil(t, agent.ImageScan)
		})
	}
}

// namespacedAgentJSON is an agent built with the namespaced Workflow named workflow.
func namespacedAgentJSON(workflow string) string {
	return `{
  "metadata": {"name": "support-bot"},
  "spec": {
    "componentType": {"name": "proxy/agent-api"},
    "owner": {"projectName": "proj"},
    "workflow": {"kind": "Workflow", "name": "` + workflow + `", "parameters": {"endpoints": []}}
  }
}`
}

// updateWithWorkflow runs UpdateComponentBuildParameters for an agent built with the namespaced
// Workflow named workflow, whose schema declares the given properties (a nil workflowSchema
// answers the Workflow read with a 500). It returns the update's error and the build
// parameters written back, nil when nothing was written.
func updateWithWorkflow(t *testing.T, workflow, workflowSchema string, imageScan *ImageScanConfig) (map[string]any, error) {
	t.Helper()
	var written map[string]any
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/workflows/") {
			if workflowSchema == "" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"boom","code":"INTERNAL"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"metadata":{"name":"` + workflow + `"},"spec":{"parameters":{"openAPIV3Schema":` + workflowSchema + `}}}`))
			return
		}
		if r.Method != http.MethodGet {
			var sent struct {
				Spec struct {
					Workflow struct {
						Parameters map[string]any `json:"parameters"`
					} `json:"workflow"`
				} `json:"spec"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			written = sent.Spec.Workflow.Parameters
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(namespacedAgentJSON(workflow)))
	}))
	err := client.UpdateComponentBuildParameters(context.Background(), "acme", "proj", "support-bot", UpdateComponentBuildParametersRequest{
		ImageScan: imageScan,
		AgentType: AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
	})
	return written, err
}

const (
	schemaWithImageScan    = `{"type":"object","properties":{"repository":{"type":"object"},"imageScan":{"type":"object"}}}`
	schemaWithoutImageScan = `{"type":"object","properties":{"repository":{"type":"object"},"docker":{"type":"object"}}}`
)

// A namespaced Workflow is read, and the setting is saved only when it declares imageScan: a
// build that would never read it (a Docker build, or an org's copy from before the setting) is
// refused with a validation error, and nothing is written.
func TestUpdateComponentBuildParameters_RefusesImageScanAWorkflowDoesNotDeclare(t *testing.T) {
	failBuild := false

	written, err := updateWithWorkflow(t, "amp-ballerina-buildpack", schemaWithImageScan, &ImageScanConfig{FailBuild: &failBuild})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"failBuild": false}, written["imageScan"])

	written, err = updateWithWorkflow(t, "amp-docker-buildpack", schemaWithoutImageScan, &ImageScanConfig{FailBuild: &failBuild})
	require.Error(t, err)
	assert.NotNil(t, utils.IsValidationError(err), "want a validation error, got %v", err)
	assert.Nil(t, written, "nothing is written when the setting is refused")
}

// Clearing the setting, or leaving it out, needs no Workflow that declares it.
func TestUpdateComponentBuildParameters_ClearsOrKeepsImageScanForAnyWorkflow(t *testing.T) {
	for name, imageScan := range map[string]*ImageScanConfig{"cleared": {}, "left out": nil} {
		t.Run(name, func(t *testing.T) {
			written, err := updateWithWorkflow(t, "amp-docker-buildpack", schemaWithoutImageScan, imageScan)
			require.NoError(t, err)
			assert.NotContains(t, written, "imageScan")
		})
	}
}

// A Workflow that can't be read leaves it unknown whether the build reads the setting, so the
// update fails rather than saving it.
func TestUpdateComponentBuildParameters_FailsWhenTheWorkflowCannotBeRead(t *testing.T) {
	failBuild := false
	written, err := updateWithWorkflow(t, "amp-ballerina-buildpack", "", &ImageScanConfig{FailBuild: &failBuild})
	require.Error(t, err)
	assert.Nil(t, utils.IsValidationError(err), "a failed read is not the caller's mistake")
	assert.Nil(t, written)
}
