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
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/websocket"
)

// stubEventHub is a hand-written EventHub stub (no generated mock exists for
// this in-package interface); it records published events and no-ops the rest.
type stubEventHub struct {
	published []eventhub.Event
}

func (s *stubEventHub) Initialize() error            { return nil }
func (s *stubEventHub) RegisterGateway(string) error { return nil }

func (s *stubEventHub) PublishEvent(_ context.Context, _ string, e eventhub.Event) error {
	s.published = append(s.published, e)
	return nil
}

func (s *stubEventHub) Subscribe(string) (<-chan eventhub.Event, error) {
	ch := make(chan eventhub.Event)
	close(ch)
	return ch, nil
}

func (s *stubEventHub) Unsubscribe(string, <-chan eventhub.Event) error { return nil }
func (s *stubEventHub) UnsubscribeAll(string) error                     { return nil }
func (s *stubEventHub) CleanUpEvents() error                            { return nil }
func (s *stubEventHub) Close() error                                    { return nil }

// stubConnChecker reports a fixed websocket-connectivity state for any gateway.
type stubConnChecker struct {
	connected bool
}

func (s *stubConnChecker) GetConnections(string) []*websocket.Connection {
	if !s.connected {
		return nil
	}
	return []*websocket.Connection{{}}
}

// apiKeyServiceFixture bundles the mocked collaborators for AgentAPIKeyService.
type apiKeyServiceFixture struct {
	svc          *AgentAPIKeyService
	apiKeyRepo   *repomocks.APIKeyRepositoryMock
	upsertedKeys *[]*models.StoredAPIKey
	lookedUpName *string
	connChecker  *stubConnChecker
	gateway      *models.Gateway
}

// newAPIKeyServiceFixture wires an AgentAPIKeyService whose artifact/env/gateway
// resolution succeeds, with GetByArtifactAndName returning the provided existing
// key (or record-not-found when nil) and Upsert recording stored keys.
func newAPIKeyServiceFixture(t *testing.T, existing *models.StoredAPIKey) *apiKeyServiceFixture {
	t.Helper()

	artifactUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	envUUID := "dada1090-0ddb-457a-a116-7f03d3cd0c4c"
	gatewayUUID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
		},
	}
	artifactRepo := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(_ string, _ string) (*models.Artifact, error) {
			return &models.Artifact{UUID: artifactUUID, Kind: models.KindAgent}, nil
		},
	}
	gateway := newGateway(t, models.GatewayRoleIngress, true)
	gateway.UUID = gatewayUUID
	gatewayRepo := gatewayFixtureRepo(t, envUUID, []*models.Gateway{gateway})

	upserted := &[]*models.StoredAPIKey{}
	lookedUp := new(string)
	apiKeyRepo := &repomocks.APIKeyRepositoryMock{
		GetByArtifactAndNameFunc: func(_ string, name string) (*models.StoredAPIKey, error) {
			*lookedUp = name
			if existing == nil {
				return nil, gorm.ErrRecordNotFound
			}
			return existing, nil
		},
		UpsertFunc: func(key *models.StoredAPIKey) error {
			*upserted = append(*upserted, key)
			return nil
		},
	}

	connChecker := &stubConnChecker{connected: true}
	svc := NewAgentAPIKeyService(artifactRepo, oc, gatewayRepo, NewGatewayEventsService(&stubEventHub{}), apiKeyRepo, connChecker)
	return &apiKeyServiceFixture{
		svc:          svc,
		apiKeyRepo:   apiKeyRepo,
		upsertedKeys: upserted,
		lookedUpName: lookedUp,
		connChecker:  connChecker,
		gateway:      gateway,
	}
}

// expectedTestKeyName mirrors the documented naming scheme: the test-key prefix
// plus the first 12 hex chars of the SHA-256 of the JWT subject.
func expectedTestKeyName(userSub string) string {
	h := sha256.Sum256([]byte(userSub))
	return models.APIKeyTestKeyPrefix + hex.EncodeToString(h[:])[:12]
}

func TestAgentAPIKeyService_IssueTestAPIKey_PerUser(t *testing.T) {
	const org, proj, agent, env = "default", "default", "customeragent", "dev"

	t.Run("creates a per-user key named from the JWT subject", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)

		resp, err := fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.APIKey)
		assert.NotEmpty(t, resp.ExpiresAt)

		want := expectedTestKeyName("user-a-sub")
		assert.Equal(t, want, *fx.lookedUpName)
		require.Len(t, *fx.upsertedKeys, 1)
		stored := (*fx.upsertedKeys)[0]
		assert.Equal(t, want, stored.Name)
		assert.Equal(t, models.APIKeyPurposeTest, stored.Purpose)
		assert.True(t, strings.HasPrefix(stored.Name, models.APIKeyTestKeyPrefix))
	})

	t.Run("different users get distinct key rows", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)

		_, err := fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")
		require.NoError(t, err)
		_, err = fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-b-sub")
		require.NoError(t, err)

		require.Len(t, *fx.upsertedKeys, 2)
		assert.NotEqual(t, (*fx.upsertedKeys)[0].Name, (*fx.upsertedKeys)[1].Name)
	})

	t.Run("reissue for the same user rotates the same row", func(t *testing.T) {
		keyName := expectedTestKeyName("user-a-sub")
		existing := &models.StoredAPIKey{
			UUID:    uuid.Must(uuid.NewV7()),
			Name:    keyName,
			Purpose: models.APIKeyPurposeTest,
		}
		fx := newAPIKeyServiceFixture(t, existing)

		resp, err := fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.APIKey)
		require.Len(t, *fx.upsertedKeys, 1)
		assert.Equal(t, keyName, (*fx.upsertedKeys)[0].Name)
	})

	t.Run("reports gateway websocket connectivity in the response", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)

		resp, err := fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")
		require.NoError(t, err)
		require.NotNil(t, resp.GatewayConnected)
		assert.True(t, *resp.GatewayConnected)

		// Only when both the local registry and is_active say otherwise is the
		// gateway reported disconnected.
		fx.connChecker.connected = false
		fx.gateway.IsActive = false
		resp, err = fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")
		require.NoError(t, err)
		require.NotNil(t, resp.GatewayConnected)
		assert.False(t, *resp.GatewayConnected)
	})

	t.Run("falls back to is_active when this replica does not hold the websocket", func(t *testing.T) {
		// A gateway holds its websocket with exactly one replica, so a replica
		// without the socket sees an empty local registry for a live gateway.
		// Reporting disconnected there is a false negative that the console acts
		// on, so is_active — written by the holding replica — decides instead.
		fx := newAPIKeyServiceFixture(t, nil)
		fx.connChecker.connected = false
		fx.gateway.IsActive = true

		resp, err := fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")
		require.NoError(t, err)
		require.NotNil(t, resp.GatewayConnected)
		assert.True(t, *resp.GatewayConnected)
	})

	t.Run("rejects reissue when the existing row is not a test key", func(t *testing.T) {
		existing := &models.StoredAPIKey{
			UUID:    uuid.Must(uuid.NewV7()),
			Name:    expectedTestKeyName("user-a-sub"),
			Purpose: models.APIKeyPurposeUserManaged,
		}
		fx := newAPIKeyServiceFixture(t, existing)

		_, err := fx.svc.IssueTestAPIKey(auditableCtx(t), org, proj, agent, env, "user-a-sub")

		assert.ErrorIs(t, err, utils.ErrBadRequest)
	})
}

func TestResolveEnvGateways_IngressOnly(t *testing.T) {
	env := uuid.New().String()
	ingress := newGateway(t, models.GatewayRoleIngress, true)
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	inactiveIngress := newGateway(t, models.GatewayRoleIngress, false)

	t.Run("returns ingress-capable only", func(t *testing.T) {
		svc := &AgentAPIKeyService{gatewayRepo: gatewayFixtureRepo(t, env, []*models.Gateway{ingress, egress})}
		got, err := svc.resolveEnvGateways(env)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, ingress.UUID, got[0].UUID)
	})

	t.Run("both counts as ingress-capable", func(t *testing.T) {
		svc := &AgentAPIKeyService{gatewayRepo: gatewayFixtureRepo(t, env, []*models.Gateway{both, egress})}
		got, err := svc.resolveEnvGateways(env)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("inactive ingress gateways still receive keys", func(t *testing.T) {
		// Keys are distributed regardless of is_active today; filtering on WebSocket
		// liveness would intermittently starve a live gateway of keys.
		svc := &AgentAPIKeyService{gatewayRepo: gatewayFixtureRepo(t, env, []*models.Gateway{inactiveIngress})}
		got, err := svc.resolveEnvGateways(env)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("egress-only environment yields ErrGatewayNotFound", func(t *testing.T) {
		svc := &AgentAPIKeyService{gatewayRepo: gatewayFixtureRepo(t, env, []*models.Gateway{egress})}
		_, err := svc.resolveEnvGateways(env)
		require.ErrorIs(t, err, utils.ErrGatewayNotFound)
	})
}

func TestAgentAPIKeyService_CreateAPIKey_ReservedTestKeyPrefix(t *testing.T) {
	const org, proj, agent, env = "default", "default", "customeragent", "dev"

	t.Run("rejects names with the console test-key prefix", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)

		_, err := fx.svc.CreateAPIKey(auditableCtx(t), org, proj, agent, env,
			&models.CreateAPIKeyRequest{Name: models.APIKeyTestKeyPrefix + "deadbeef0123"})

		assert.ErrorIs(t, err, utils.ErrBadRequest)
	})

	t.Run("reports gateway websocket connectivity in the response", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)
		fx.connChecker.connected = false
		fx.gateway.IsActive = false

		resp, err := fx.svc.CreateAPIKey(auditableCtx(t), org, proj, agent, env,
			&models.CreateAPIKeyRequest{Name: "my-key"})

		require.NoError(t, err)
		require.NotNil(t, resp.GatewayConnected)
		assert.False(t, *resp.GatewayConnected)
	})

	t.Run("falls back to is_active when this replica does not hold the websocket", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)
		fx.connChecker.connected = false
		fx.gateway.IsActive = true

		resp, err := fx.svc.CreateAPIKey(auditableCtx(t), org, proj, agent, env,
			&models.CreateAPIKeyRequest{Name: "my-key"})

		require.NoError(t, err)
		require.NotNil(t, resp.GatewayConnected)
		assert.True(t, *resp.GatewayConnected)
	})
}

func TestAgentAPIKeyService_CreateAPIKey_MinNameLength(t *testing.T) {
	const org, proj, agent, env = "default", "default", "customeragent", "dev"

	rejected := map[string]*models.CreateAPIKeyRequest{
		"explicit name":                     {Name: "ab"},
		"name derived from display name":    {DisplayName: "ab"},
		"display name that sanitizes short": {DisplayName: "a!!"},
	}
	for desc, req := range rejected {
		t.Run("rejects a too-short "+desc+" without storing the key", func(t *testing.T) {
			fx := newAPIKeyServiceFixture(t, nil)

			_, err := fx.svc.CreateAPIKey(auditableCtx(t), org, proj, agent, env, req)

			assert.ErrorIs(t, err, utils.ErrBadRequest)
			assert.Empty(t, *fx.upsertedKeys)
		})
	}

	t.Run("accepts a name at the minimum length", func(t *testing.T) {
		fx := newAPIKeyServiceFixture(t, nil)

		_, err := fx.svc.CreateAPIKey(auditableCtx(t), org, proj, agent, env,
			&models.CreateAPIKeyRequest{DisplayName: "abc"})

		require.NoError(t, err)
		require.Len(t, *fx.upsertedKeys, 1)
		assert.Equal(t, "abc", (*fx.upsertedKeys)[0].Name)
	})
}
