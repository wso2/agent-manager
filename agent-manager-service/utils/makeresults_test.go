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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestConvertToConfigurations_SurfacesInstrumentationVersion(t *testing.T) {
	version := "0.3.0"
	got := convertToConfigurations(&models.Configurations{
		InstrumentationVersion: &version,
	})
	if got == nil {
		t.Fatal("expected non-nil configurations")
	}
	// The read path must surface the pinned version, otherwise the deploy/promote
	// UI falls back to the platform default and shows a version different from
	// what is actually deployed.
	if !got.InstrumentationVersion.IsSet() {
		t.Fatal("InstrumentationVersion should be set on the response")
	}
	if v := got.InstrumentationVersion.Get(); v == nil || *v != "0.3.0" {
		t.Errorf("InstrumentationVersion = %v, want 0.3.0", v)
	}
}

func TestConvertToConfigurations_UnpinnedIsOmitted(t *testing.T) {
	got := convertToConfigurations(&models.Configurations{})
	if got == nil {
		t.Fatal("expected non-nil configurations")
	}
	if got.InstrumentationVersion.IsSet() {
		t.Errorf("InstrumentationVersion should be unset when the agent has no pin")
	}
}

// The kind name and version travel together: a response carrying one but not the
// other leaves every consumer unable to say which published version an agent runs.
func TestConvertToAgentResponse_SurfacesKindVersion(t *testing.T) {
	kindAgent := func(kindName, kindVersion string) *models.AgentResponse {
		return &models.AgentResponse{
			Name:         "kind-agent",
			Provisioning: models.Provisioning{Type: string(InternalAgent)},
			KindName:     kindName,
			KindVersion:  kindVersion,
		}
	}

	got := ConvertToAgentResponse(kindAgent("bal-task-assistant", "1.0.1"))
	if got.KindName == nil || *got.KindName != "bal-task-assistant" {
		t.Errorf("KindName = %v, want bal-task-assistant", got.KindName)
	}
	if got.KindVersion == nil || *got.KindVersion != "1.0.1" {
		t.Errorf("KindVersion = %v, want 1.0.1", got.KindVersion)
	}

	// Agents created before the version was recorded must omit the field rather
	// than report an empty version.
	got = ConvertToAgentResponse(kindAgent("bal-task-assistant", ""))
	if got.KindVersion != nil {
		t.Errorf("KindVersion = %v, want nil for an agent with no recorded version", *got.KindVersion)
	}
}

// The deployed kind version is what the environment actually runs; dropping it in
// the mapper would leave the console re-deriving it from image IDs.
func TestConvertToDeploymentDetailsResponse_SurfacesKindVersion(t *testing.T) {
	got := ConvertToDeploymentDetailsResponse([]*models.DeploymentResponse{
		{Environment: "default", ImageId: "img:v1", KindVersion: "1.2.0"},
		{Environment: "prod", ImageId: "img:v0"},
	})

	deployed, ok := got["default"]
	if !ok {
		t.Fatal("expected a response for the default environment")
	}
	if deployed.KindVersion == nil || *deployed.KindVersion != "1.2.0" {
		t.Errorf("KindVersion = %v, want 1.2.0", deployed.KindVersion)
	}

	// An image matching no published version must omit the field rather than
	// report an empty version.
	unresolved, ok := got["prod"]
	if !ok {
		t.Fatal("expected a response for the prod environment")
	}
	if unresolved.KindVersion != nil {
		t.Errorf("KindVersion = %v, want nil when the image matches no version", *unresolved.KindVersion)
	}
}

func TestPopulateConfigurationResponseReportsInheritedCardCORS(t *testing.T) {
	resp := &spec.ConfigurationResponse{}
	PopulateConfigurationResponseFromAgentConfig(resp, &models.AgentConfig{CORSEnabled: true, CORSAllowOrigins: []string{"*"}})
	require.NotNil(t, resp.AgentCardCorsConfig)
	assert.True(t, resp.AgentCardCorsConfig.GetInherit())
	assert.True(t, resp.AgentCardCorsConfig.GetEnabled())
	assert.Equal(t, []string{"*"}, resp.AgentCardCorsConfig.AllowOrigin)
	assert.Equal(t, []string{"Content-Type"}, resp.AgentCardCorsConfig.AllowHeaders)
}
