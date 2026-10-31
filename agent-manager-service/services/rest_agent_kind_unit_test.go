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

package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

var restTestEnvUUID = uuid.MustParse("11111111-2222-3333-4444-555555555555")

const restTestServiceURL = "http://c.ns.svc.cluster.local:8000/api"

type restKindFixture struct {
	kind     *RestAgentKind
	exposure *models.PlatformAgentExposure
	url      string
	cfg      *models.AgentConfig
	deployed []byte
}

// newRestKindFixture is an agent "a" in project "p" whose exposure exists and whose release binding serves
// restTestServiceURL, with an empty environment config and nothing deployed.
func newRestKindFixture() *restKindFixture {
	f := &restKindFixture{url: restTestServiceURL, cfg: &models.AgentConfig{}}
	f.exposure = &models.PlatformAgentExposure{
		OUID: "ou-1", ProjectName: "p", AgentName: "a", EnvironmentName: "dev",
		EnvironmentUUID: restTestEnvUUID, Protocol: models.AgentExposureProtocolREST,
	}
	exposures := &repomocks.PlatformAgentExposureRepositoryMock{
		GetByArtifactFunc: func(context.Context, string, uuid.UUID) (*models.PlatformAgentExposure, error) {
			if f.exposure == nil {
				return nil, repositories.ErrPlatformAgentExposureNotFound
			}
			return f.exposure, nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetReleaseBindingServiceURLFunc: func(context.Context, string, string, string) (string, error) { return f.url, nil },
	}
	configs := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) { return f.cfg, nil },
	}
	deployments := &repomocks.DeploymentRepositoryMock{
		GetPullableFunc: func(context.Context, string, uuid.UUID, string) (*models.Deployment, error) {
			if f.deployed == nil {
				return nil, nil //nolint:nilnil // nothing deployed yet
			}
			return &models.Deployment{Content: f.deployed}, nil
		},
	}
	f.kind = NewRestAgentKind(NewPlatformAgentExposures(exposures, nil, oc), configs, deployments, testLogger())
	return f
}

func restTestArtifact() models.ArtifactRef {
	return models.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: models.KindRestAPI}
}

func (f *restKindFixture) render(t *testing.T) RestAPIDeploymentYAML {
	t.Helper()
	rendered, err := f.kind.Render(context.Background(), restTestArtifact())
	require.NoError(t, err)
	var content RestAPIDeploymentYAML
	require.NoError(t, yaml.Unmarshal(rendered.Content, &content))
	return content
}

func TestRestAgentRenderWaitsForEndpoint(t *testing.T) {
	f := newRestKindFixture()
	f.url = ""

	_, err := f.kind.Render(context.Background(), restTestArtifact())

	require.ErrorIs(t, err, reconcile.ErrInputNotReady)
	assert.Equal(t, "waiting for the agent's endpoint", err.Error())
}

func TestRestAgentRenderRecordGone(t *testing.T) {
	f := newRestKindFixture()
	f.exposure = nil

	_, err := f.kind.Render(context.Background(), restTestArtifact())

	require.ErrorIs(t, err, reconcile.ErrRecordGone)
}

func TestRestAgentRenderUpstreamFromServiceURL(t *testing.T) {
	timeout := int32(45)
	f := newRestKindFixture()
	f.cfg = &models.AgentConfig{ResilienceTimeoutSeconds: &timeout}

	content := f.render(t)

	assert.Equal(t, "gateway.api-platform.wso2.com/v1", content.ApiVersion)
	assert.Equal(t, "RestApi", content.Kind)
	assert.Equal(t, agentGatewayName("p", "a", restTestEnvUUID.String()), content.Metadata.Name)
	assert.Equal(t, "a", content.Spec.DisplayName)
	assert.Equal(t, "v1.0", content.Spec.Version)
	assert.Equal(t, "/a", content.Spec.Context)
	assert.Equal(t, restTestServiceURL, content.Spec.Upstream.Main.URL)
	assert.Equal(t, []Operation{
		{Method: "GET", Path: "/*"},
		{Method: "POST", Path: "/*"},
		{Method: "PUT", Path: "/*"},
		{Method: "DELETE", Path: "/*"},
		{Method: "PATCH", Path: "/*"},
		{Method: "OPTIONS", Path: "/*"},
	}, content.Spec.Operations)
	require.NotNil(t, content.Spec.Resilience)
	assert.Equal(t, "45s", *content.Spec.Resilience.Timeout)
	assert.Equal(t, "0s", *content.Spec.Resilience.IdleTimeout)
}

func TestRestAgentRenderDefaultResilienceTimeout(t *testing.T) {
	content := newRestKindFixture().render(t)

	assert.Equal(t, client.FormatResilienceTimeout(client.DefaultResilienceTimeoutSeconds), *content.Spec.Resilience.Timeout)
}

func TestRestAgentRenderPolicies(t *testing.T) {
	f := newRestKindFixture()
	f.cfg = &models.AgentConfig{
		EnableApiKeySecurity: true, CORSEnabled: true,
		CORSAllowOrigins: []string{"https://client.example"}, CORSAllowMethods: []string{"GET"}, CORSAllowHeaders: []string{"X-API-Key"},
	}

	configured := f.render(t)
	plain := newRestKindFixture().render(t)

	want := buildPolicies(resolveAPIConfig(f.cfg, nil, nil, nil, nil, false))
	require.Len(t, want, 2)
	assert.Equal(t, "cors", want[0]["name"], "CORS runs before the key check")
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(configured.Spec.Policies)
	assert.JSONEq(t, string(wantJSON), string(gotJSON))
	assert.Equal(t, []map[string]interface{}{}, plain.Spec.Policies)
}

func TestRestAgentEvents(t *testing.T) {
	performedAt := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	artifactUUID := uuid.MustParse("0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f")
	ref := models.ArtifactRef{ArtifactUUID: artifactUUID}
	kind := &RestAgentKind{}
	payloadOf := func(evt eventhub.Event) (string, map[string]any) {
		var envelope struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
		return envelope.Type, envelope.Payload
	}

	deployed := kind.DeployEvent(reconcile.EventRef{
		Placement: models.Placement{ArtifactRef: ref}, DeploymentID: "dep-1", PerformedAt: performedAt,
	})
	undeployed := kind.UndeployEvent(reconcile.EventRef{Placement: models.Placement{ArtifactRef: ref}, PerformedAt: performedAt})
	deleted := kind.DeleteEvent(ref)

	assert.Equal(t, eventhub.EventType("api.deployed"), deployed.EventType)
	typ, payload := payloadOf(deployed)
	assert.Equal(t, "api.deployed", typ)
	assert.Equal(t, artifactUUID.String(), payload["apiId"])
	assert.Equal(t, "dep-1", payload["deploymentId"])

	assert.Equal(t, eventhub.EventType("api.undeployed"), undeployed.EventType)
	_, payload = payloadOf(undeployed)
	assert.Equal(t, artifactUUID.String(), payload["apiId"])
	deploymentID, present := payload["deploymentId"]
	assert.True(t, present, "the gateway's payload carries deploymentId even when empty")
	assert.Equal(t, "", deploymentID)

	assert.Equal(t, eventhub.EventType("api.deleted"), deleted.EventType)
	_, payload = payloadOf(deleted)
	assert.Equal(t, map[string]any{"apiId": artifactUUID.String()}, payload)
}

func TestRestAgentDrifted(t *testing.T) {
	placement := models.Placement{ArtifactRef: restTestArtifact(), GatewayUUID: uuid.New()}
	deployedAt := func(url string) []byte {
		content, err := yaml.Marshal(RestAPIDeploymentYAML{Spec: RestAPIDeploymentSpec{
			Upstream: RestAPIUpstream{Main: RestAPIUpstreamTarget{URL: url}},
		}})
		require.NoError(t, err)
		return content
	}

	t.Run("another endpoint is drift", func(t *testing.T) {
		f := newRestKindFixture()
		f.deployed = deployedAt("http://old.ns.svc.cluster.local:8000")

		reason, drifted, err := f.kind.Drifted(context.Background(), placement)

		require.NoError(t, err)
		assert.True(t, drifted)
		assert.Equal(t, "the agent's endpoint changed", reason)
	})
	t.Run("the same endpoint is not", func(t *testing.T) {
		f := newRestKindFixture()
		f.deployed = deployedAt(restTestServiceURL)

		_, drifted, err := f.kind.Drifted(context.Background(), placement)

		require.NoError(t, err)
		assert.False(t, drifted)
	})
	t.Run("no exposure is not", func(t *testing.T) {
		f := newRestKindFixture()
		f.exposure = nil

		_, drifted, err := f.kind.Drifted(context.Background(), placement)

		require.NoError(t, err)
		assert.False(t, drifted)
	})
	t.Run("an endpoint not ready is not", func(t *testing.T) {
		f := newRestKindFixture()
		f.url = ""
		f.deployed = deployedAt(restTestServiceURL)

		_, drifted, err := f.kind.Drifted(context.Background(), placement)

		require.NoError(t, err)
		assert.False(t, drifted)
	})
}
