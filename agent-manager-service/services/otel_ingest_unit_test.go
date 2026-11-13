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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestOtelIngestKindRendersTheIngestAPI(t *testing.T) {
	kind := NewOtelIngestKind("http://otel-collector:4318", "agent-manager-service")
	rendered, err := kind.Render(context.Background(), models.ArtifactRef{})
	require.NoError(t, err)

	var api RestAPIDeploymentYAML
	require.NoError(t, yaml.Unmarshal(rendered.Content, &api))
	assert.Equal(t, models.KindRestAPI, api.Kind)
	assert.Equal(t, "/otel", api.Spec.Context)
	assert.Equal(t, "http://otel-collector:4318", api.Spec.Upstream.Main.URL)
	assert.Equal(t, []Operation{{Method: "POST", Path: "/v1/traces"}}, api.Spec.Operations)
	require.Len(t, api.Spec.Policies, 1)
	params, _ := api.Spec.Policies[0]["params"].(map[string]interface{})
	assert.Equal(t, "jwt-auth", api.Spec.Policies[0]["name"])
	assert.Equal(t, "x-amp-api-key", params["headerName"])
	assert.Equal(t, []interface{}{"agent-manager-service"}, params["issuers"])
	claims, _ := params["claimMappings"].(map[string]interface{})
	assert.Equal(t, "x-user-organization-name", claims["organization"])
	assert.Equal(t, "x-user-agent-name", claims["agent"])

	again, err := kind.Render(context.Background(), models.ArtifactRef{})
	require.NoError(t, err)
	assert.Equal(t, rendered, again, "the render must be stable so a connecting gateway is not redeployed")
}

func TestOtelIngestKindNeedsAnUpstream(t *testing.T) {
	_, err := NewOtelIngestKind(" ", "iss").Render(context.Background(), models.ArtifactRef{})
	require.Error(t, err)
}

func TestOtelIngestKindIsARestAPIOnTheGateway(t *testing.T) {
	core := reconcile.NewCore(nil, nil, nil, reconcile.Kinds{
		NewOtelIngestKind("http://c:4318", "iss"),
	}, reconcile.Config{}, discardLogger())
	id := uuid.New()
	kind, artifactID, ok := core.GatewayArtifact(models.ArtifactRef{ArtifactUUID: id, Kind: models.KindOtelIngest})
	assert.True(t, ok)
	assert.Equal(t, models.KindRestAPI, kind)
	assert.Equal(t, id.String(), artifactID)
}

func TestOtelIngestPublisherDisabledDoesNothing(t *testing.T) {
	// nil repositories and core: any use panics.
	p := NewOtelIngestPublisher(nil, NewOtelIngestKind("", "iss"), nil, nil, nil, discardLogger())
	assert.False(t, p.Enabled())
	require.NoError(t, p.EnsureOnGateway(context.Background(), seedOUID, uuid.New()))
}

func TestOtelIngestPublisherSkipsAGatewayServingTheCurrentAPI(t *testing.T) {
	kind := NewOtelIngestKind("http://otel-collector:4318", "iss")
	rendered, err := kind.Render(context.Background(), models.ArtifactRef{})
	require.NoError(t, err)
	artifact := &models.Artifact{UUID: uuid.New(), Handle: otelIngestHandle, Kind: models.KindOtelIngest, OUID: seedOUID}
	gateway := uuid.New()
	artifacts := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, org string) (*models.Artifact, error) { return artifact, nil },
	}
	deployments := &repomocks.DeploymentRepositoryMock{
		GetPullableFunc: func(ctx context.Context, ouID string, a uuid.UUID, gw string) (*models.Deployment, error) {
			assert.Equal(t, artifact.UUID, a)
			assert.Equal(t, gateway.String(), gw)
			return &models.Deployment{Content: rendered.Content}, nil
		},
	}
	// nil db and core: a redeploy would panic.
	p := NewOtelIngestPublisher(nil, kind, artifacts, deployments, nil, discardLogger())
	require.NoError(t, p.EnsureOnGateway(context.Background(), seedOUID, gateway))
}

func TestOtelIngestPublisherRefusesAHandleTakenByAnotherKind(t *testing.T) {
	artifacts := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, org string) (*models.Artifact, error) {
			return &models.Artifact{UUID: uuid.New(), Handle: handle, Kind: models.KindLLMProxy}, nil
		},
	}
	p := NewOtelIngestPublisher(nil, NewOtelIngestKind("http://c:4318", "iss"), artifacts, nil, nil, discardLogger())
	require.ErrorContains(t, p.EnsureOnGateway(context.Background(), seedOUID, uuid.New()), "taken")
}
