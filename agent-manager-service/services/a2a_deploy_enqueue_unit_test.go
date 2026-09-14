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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// A deploy records the intent to publish rather than publishing inline: the
// upstream address is not readable until the release binding reconciles.
func TestEnqueueA2APublicationRecordsTheArtifactUUID(t *testing.T) {
	var enqueued []models.A2APublication
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}

	envUUID := uuid.New()
	artifactUUID := uuid.New()
	svc.enqueueA2APublication(context.Background(), "org-1", "checkout", "trip-planner", "dev", envUUID, artifactUUID)

	require.Len(t, enqueued, 1)
	assert.Equal(t, "trip-planner", enqueued[0].AgentName)
	assert.Equal(t, "dev", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].ArtifactUUID)
}

// A queue failure must not fail a deploy that has otherwise succeeded — the
// reconciler is not the deploy's critical path, and the agent is running.
func TestEnqueueA2APublicationSurvivesRepoFailure(t *testing.T) {
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			return assert.AnError
		},
	}
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}
	assert.NotPanics(t, func() {
		svc.enqueueA2APublication(context.Background(), "org-1", "checkout", "trip-planner", "dev", uuid.New(), uuid.New())
	})
}
