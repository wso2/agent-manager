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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// cardServiceFor builds the service over an agent of the given provisioning and subtype.
const cardTestEnvUUID = "55555555-5555-5555-5555-555555555555"

func cardServiceFor(provisioning, subType string) (*a2aAgentCardService, *repomocks.A2AAgentCardRepositoryMock, *clientmocks.OpenChoreoClientMock) {
	oc := &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID: "agent-uuid", Name: name,
				Provisioning: models.Provisioning{Type: provisioning},
				Type:         models.AgentType{SubType: subType},
			}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: cardTestEnvUUID, Name: name}, nil
		},
	}
	repo := &repomocks.A2AAgentCardRepositoryMock{
		EnqueueFunc:           func(context.Context, *models.A2AAgentCard) error { return nil },
		RequeueFunc:           func(context.Context, string, string, string, string) error { return nil },
		DeleteForAgentEnvFunc: func(context.Context, string, string, string, string) error { return nil },
	}
	return NewA2AAgentCardService(oc, repo, testLogger()).(*a2aAgentCardService), repo, oc
}

func TestGetA2AAgentCardReturnsTheStoredRow(t *testing.T) {
	svc, repo, oc := cardServiceFor("internal", "a2a-agent")
	stored := &models.A2AAgentCard{Status: models.A2AAgentCardStatusFetched, Card: json.RawMessage(validTestCard)}
	repo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return stored, nil
	}
	oc.GetEnvironmentFunc = nil // a stored row proves the environment
	oc.GetComponentFunc = nil   // and the agent, so a 5s poll costs no OpenChoreo call

	got, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	require.NoError(t, err)
	assert.Same(t, stored, got)
}

// No row means no fetch is queued, so a synthetic pending would be polled forever.
func TestGetA2AAgentCardIsNotFoundForAPlatformAgentWithNoRow(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return nil, repositories.ErrA2AAgentCardNotFound
	}

	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentCardNotFound)
	assert.Empty(t, repo.EnqueueCalls(), "a read never queues a fetch")
}

func TestGetA2AAgentCardIsNotFoundForAnExternalAgentWithNoSource(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return nil, repositories.ErrA2AAgentCardNotFound
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentCardNotFound)
}

func TestGetA2AAgentCardIsNotFoundForANonA2AAgent(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "chat-api")
	repo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return nil, repositories.ErrA2AAgentCardNotFound
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentCardNotFound)
}

func TestGetA2AAgentCardDoesNotMaskRealErrors(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return nil, assert.AnError
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, utils.ErrAgentCardNotFound)
}

func TestGetA2AAgentCardMapsAMissingAgent(t *testing.T) {
	svc, repo, oc := cardServiceFor("internal", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return nil, repositories.ErrA2AAgentCardNotFound
	}
	oc.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return nil, utils.ErrNotFound
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentNotFound)
}

func TestRefreshA2AAgentCardQueuesAPlatformFetch(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "a2a-agent")
	require.NoError(t, svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev"))
	queued := repo.EnqueueCalls()
	require.Len(t, queued, 1)
	assert.Equal(t, models.A2AAgentCardSourcePlatform, queued[0].Card.Source)
	assert.Empty(t, queued[0].Card.SourceURL)
	assert.Equal(t, uuid.MustParse(cardTestEnvUUID), queued[0].Card.EnvironmentUUID)
}

func TestRefreshA2AAgentCardRequeuesTheExternalRowInPlace(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	repo.EnqueueFunc = nil // an upsert could recreate a row removed meanwhile
	require.NoError(t, svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev"))
	requeued := repo.RequeueCalls()
	require.Len(t, requeued, 1)
	assert.Equal(t, "dev", requeued[0].EnvironmentName)
}

func TestRefreshA2AAgentCardNeedsAnExternalSource(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	repo.RequeueFunc = func(context.Context, string, string, string, string) error {
		return repositories.ErrA2AAgentCardNotFound
	}
	err := svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentCardNotFound)
	assert.Empty(t, repo.EnqueueCalls())
}

func TestRefreshA2AAgentCardDoesNotMaskARequeueFailure(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	repo.RequeueFunc = func(context.Context, string, string, string, string) error { return assert.AnError }
	err := svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, utils.ErrAgentCardNotFound)
}

func TestRefreshA2AAgentCardRejectsANonA2AAgent(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "custom-api")
	err := svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentNotA2A)
	assert.Empty(t, repo.EnqueueCalls())
}

func TestRefreshA2AAgentCardEnforcesTheEnvironmentTier(t *testing.T) {
	svc, repo, oc := cardServiceFor("internal", "a2a-agent")
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: name, IsProduction: true}, nil
	}
	err := svc.RefreshA2AAgentCard(tierCtx(t, rbac.AgentEnvNonProduction), "org", "proj", "agent", "prod")
	assert.ErrorIs(t, err, utils.ErrForbidden)
	assert.Empty(t, repo.EnqueueCalls())
}

func TestSetA2ACardSourceQueuesTheExternalURL(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	const url = "https://93.184.215.14/.well-known/agent-card.json"

	require.NoError(t, svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", "  "+url+" "))

	queued := repo.EnqueueCalls()
	require.Len(t, queued, 1)
	assert.Equal(t, models.A2AAgentCardSourceExternal, queued[0].Card.Source)
	assert.Equal(t, url, queued[0].Card.SourceURL)
	assert.Equal(t, "dev", queued[0].Card.EnvironmentName)
	assert.Equal(t, uuid.MustParse(cardTestEnvUUID), queued[0].Card.EnvironmentUUID)
}

func TestSetA2ACardSourceRejects(t *testing.T) {
	cases := []struct {
		name, provisioning, subType, url string
		want                             error
	}{
		{"non-A2A agent", "external", "custom-api", "https://93.184.215.14/c", utils.ErrAgentNotA2A},
		{"platform agent", "internal", "a2a-agent", "https://93.184.215.14/c", utils.ErrAgentCardSourceNotExternal},
		{"loopback URL", "external", "a2a-agent", "http://127.0.0.1/c", utils.ErrInvalidURL},
		{"non-http scheme", "external", "a2a-agent", "ftp://93.184.215.14/c", utils.ErrInvalidURL},
		{"empty URL", "external", "a2a-agent", "   ", utils.ErrInvalidURL},
		{"too long", "external", "a2a-agent", "https://93.184.215.14/" + strings.Repeat("a", 2048), utils.ErrInvalidURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := cardServiceFor(tc.provisioning, tc.subType)
			err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", tc.url)
			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, repo.EnqueueCalls())
		})
	}
}

func TestSetA2ACardSourceRejectsAnUnknownEnvironment(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	oc.GetEnvironmentFunc = func(context.Context, string, string) (*models.EnvironmentResponse, error) {
		return nil, utils.ErrNotFound
	}
	err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "nope", "https://93.184.215.14/c")
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	assert.Empty(t, repo.EnqueueCalls())
}

func TestDeleteA2ACardSourceRemovesThatEnvironmentsRow(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	require.NoError(t, svc.DeleteA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev"))
	deleted := repo.DeleteForAgentEnvCalls()
	require.Len(t, deleted, 1)
	assert.Equal(t, "dev", deleted[0].EnvironmentName)
}

func TestDeleteA2ACardSourceRejectsAPlatformAgent(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "a2a-agent")
	err := svc.DeleteA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, utils.ErrAgentCardSourceNotExternal)
	assert.Empty(t, repo.DeleteForAgentEnvCalls())
}

// agentGoneDuringEnqueue swaps the agent lookup the moment the row is written, as a racing delete would.
func agentGoneDuringEnqueue(repo *repomocks.A2AAgentCardRepositoryMock, oc *clientmocks.OpenChoreoClientMock, after func(context.Context, string, string, string) (*models.AgentResponse, error)) {
	repo.EnqueueFunc = func(context.Context, *models.A2AAgentCard) error {
		oc.GetComponentFunc = after
		return nil
	}
}

func TestSetA2ACardSourceDropsTheRowWhenTheAgentIsDeletedMeanwhile(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	agentGoneDuringEnqueue(repo, oc, func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return nil, utils.ErrNotFound
	})

	err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", "https://93.184.215.14/.well-known/agent-card.json")

	assert.ErrorIs(t, err, utils.ErrAgentNotFound)
	deleted := repo.DeleteForAgentEnvCalls()
	require.Len(t, deleted, 1)
	assert.Equal(t, "dev", deleted[0].EnvironmentName)
}

func TestRefreshA2AAgentCardDropsTheRowWhenTheAgentIsReplacedMeanwhile(t *testing.T) {
	svc, repo, oc := cardServiceFor("internal", "a2a-agent")
	agentGoneDuringEnqueue(repo, oc, func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{UUID: "new-agent-uuid", Name: name, Type: models.AgentType{SubType: "a2a-agent"}}, nil
	})

	err := svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev")

	assert.ErrorIs(t, err, utils.ErrAgentNotFound)
	assert.Len(t, repo.DeleteForAgentEnvCalls(), 1)
}

func TestSetA2ACardSourceDoesNotMaskARecheckFailure(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	agentGoneDuringEnqueue(repo, oc, func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return nil, assert.AnError
	})

	err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", "https://93.184.215.14/.well-known/agent-card.json")

	assert.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrAgentNotFound)
	assert.Empty(t, repo.DeleteForAgentEnvCalls())
}

// Unresolvable and internal hosts must look alike, or the reply is an internal-DNS oracle.
func TestSetA2ACardSourceHidesWhyAHostIsRefused(t *testing.T) {
	svc, _, _ := cardServiceFor("external", "a2a-agent")
	ctx := tierGrantedCtx(t)

	internal := svc.SetA2ACardSource(ctx, "org", "proj", "agent", "dev", "https://10.0.0.1/c")
	unresolvable := svc.SetA2ACardSource(ctx, "org", "proj", "agent", "dev", "https://nope.invalid/c")

	require.ErrorIs(t, internal, utils.ErrInvalidURL)
	require.ErrorIs(t, unresolvable, utils.ErrInvalidURL)
	assert.Equal(t, internal.Error(), unresolvable.Error())
	assert.NotContains(t, unresolvable.Error(), "lookup")
}

// Only callers allowed on the environment may probe URLs through it.
func TestSetA2ACardSourceChecksTheTierBeforeTheURL(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: name, IsProduction: true}, nil
	}
	err := svc.SetA2ACardSource(tierCtx(t, rbac.AgentEnvNonProduction), "org", "proj", "agent", "prod", "https://10.0.0.1/c")
	assert.ErrorIs(t, err, utils.ErrForbidden)
	assert.Empty(t, repo.EnqueueCalls())
}

// The OpenAPI maxLength and the console both count characters, not bytes.
func TestSetA2ACardSourceCountsURLLengthInCharacters(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	base := "https://93.184.215.14/"
	url := base + strings.Repeat("é", 2048-len(base))

	require.NoError(t, svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", url))
	assert.Len(t, repo.EnqueueCalls(), 1)
}

// A row keyed on a zero environment UUID could never be told apart from another environment's.
func TestSetA2ACardSourceRefusesAnEnvironmentWithoutAUUID(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	repo.EnqueueFunc = nil
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: "not-a-uuid", Name: name}, nil
	}

	err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", "https://93.184.215.14/.well-known/agent-card.json")

	require.Error(t, err)
}
