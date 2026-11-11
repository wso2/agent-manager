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

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var restTestEnvUUID = uuid.MustParse("11111111-2222-3333-4444-555555555555")

type restKindFixture struct {
	kind     *RestAgentKind
	exposure *models.PlatformAgentExposure
	cfg      *models.AgentConfig
	deployed []byte
}

// newRestKindFixture is an agent "a" in project "p" whose exposure exists, with an empty environment config and
// nothing deployed.
func newRestKindFixture() *restKindFixture {
	f := &restKindFixture{cfg: &models.AgentConfig{}}
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
	oc := &repomocks.PlatformRepositoryMock{}
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
	f.kind = NewRestAgentKind(NewPlatformAgentExposures(exposures, nil, oc), configs, deployments, nil, nil, testLogger())
	return f
}

func restTestArtifact() models.ArtifactRef {
	return models.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: models.KindRestAPI}
}

// A platform-hosted agent needs an agent runtime to serve its endpoint, which is not available.
func TestRestAgentRenderHasNoHostedEndpoint(t *testing.T) {
	f := newRestKindFixture()

	_, err := f.kind.Render(context.Background(), restTestArtifact())

	require.ErrorIs(t, err, utils.ErrNotImplemented)
}

func TestRestAgentRenderRecordGone(t *testing.T) {
	f := newRestKindFixture()
	f.exposure = nil

	_, err := f.kind.Render(context.Background(), restTestArtifact())

	require.ErrorIs(t, err, reconcile.ErrRecordGone)
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

// An artifact with no exposure has nothing to drift from.
func TestRestAgentDriftedWithoutExposure(t *testing.T) {
	placement := models.Placement{ArtifactRef: restTestArtifact(), GatewayUUID: uuid.New()}
	f := newRestKindFixture()
	f.exposure = nil

	_, drifted, err := f.kind.Drifted(context.Background(), placement)

	require.NoError(t, err)
	assert.False(t, drifted)
}
