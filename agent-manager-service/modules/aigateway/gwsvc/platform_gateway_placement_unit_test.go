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

package gwsvc

import (
	"context"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
)

type placedEnv struct {
	ouID string
	env  uuid.UUID
}

type recordingPlacer chan placedEnv

func (p recordingPlacer) RequestPlaceEnv(ouID string, env uuid.UUID) {
	p <- placedEnv{ouID: ouID, env: env}
}

// placed waits for the placements a mapping triggers after its transaction commits.
func (p recordingPlacer) placed(t *testing.T, n int) []placedEnv {
	t.Helper()
	var got []placedEnv
	for range n {
		select {
		case placed := <-p:
			got = append(got, placed)
		case <-time.After(5 * time.Second):
			t.Fatalf("got %d placements, want %d", len(got), n)
		}
	}
	return got
}

func mappingRepo(gw *gwmodels.Gateway, ingressInEnv int64) *gwrepomocks.GatewayRepositoryMock {
	return &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc:                 func(string, string) (*gwmodels.Gateway, error) { return nil, gwerr.ErrGatewayNotFound },
		GetByUUIDFunc:                         func(string) (*gwmodels.Gateway, error) { return gw, nil },
		TransactionFunc:                       func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		CreateTxFunc:                          func(*gorm.DB, *gwmodels.Gateway) error { return nil },
		AcquireGatewayLockFunc:                func(*gorm.DB, string) error { return nil },
		AcquireEnvironmentLockFunc:            func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:          func(string, string) (bool, error) { return false, nil },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) { return nil, nil },
		CountIngressCapableInEnvironmentFunc:  func(*gorm.DB, string) (int64, error) { return ingressInEnv, nil },
		CreateEnvironmentMappingTxFunc:        func(*gorm.DB, *gwmodels.GatewayEnvironmentMapping) error { return nil },
	}
}

func TestAssignGatewayToEnvironmentPlacesTheEnvironment(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: uuid.New(), OUID: "ou-1", GatewayFunctionalityType: gwmodels.GatewayRoleBoth}
	env := uuid.New()
	placer := make(recordingPlacer, 1)
	svc := NewPlatformGatewayService(mappingRepo(gw, 0)).WithEnvironmentPlacer(placer)

	require.NoError(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), env.String()))

	assert.Equal(t, []placedEnv{{ouID: "ou-1", env: env}}, placer.placed(t, 1))
}

func TestAssignGatewayToEnvironmentPlacesNothingWhenTheMappingFails(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: uuid.New(), OUID: "ou-1", GatewayFunctionalityType: gwmodels.GatewayRoleBoth}
	placer := make(recordingPlacer, 1)
	svc := NewPlatformGatewayService(mappingRepo(gw, 1)).WithEnvironmentPlacer(placer)

	require.ErrorIs(t, svc.AssignGatewayToEnvironment(gw.UUID.String(), uuid.NewString()), gwerr.ErrGatewayIngressCapExceeded)

	select {
	case placed := <-placer:
		t.Fatalf("placed %v after a rejected mapping", placed)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRegisterGatewayPlacesItsEnvironment(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleEgress}
	env := uuid.New()
	placer := make(recordingPlacer, 1)
	svc := NewPlatformGatewayService(mappingRepo(gw, 0)).WithEnvironmentPlacer(placer)

	_, err := svc.RegisterGateway("ou-1", "gw1", "GW", "", "https://ext.example.com", "", false, "EGRESS", nil,
		[]string{env.String(), env.String()}, gwmodels.GatewayHostingManaged)
	require.NoError(t, err)

	assert.Equal(t, []placedEnv{{ouID: "ou-1", env: env}}, placer.placed(t, 1))
}

func TestSelfHostedGatewayDoesNotChangeManagedPlacement(t *testing.T) {
	env := uuid.New()
	managed := &gwmodels.Gateway{
		UUID: uuid.New(), Name: "managed", GatewayFunctionalityType: gwmodels.GatewayRoleBoth,
		Hosting: gwmodels.GatewayHostingManaged,
	}
	selfHosted := &gwmodels.Gateway{
		UUID: uuid.New(), Name: "self-hosted", GatewayFunctionalityType: gwmodels.GatewayRoleEgress,
		Hosting: gwmodels.GatewayHostingSelfHosted,
	}
	repo := &gwrepomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
			return []*gwmodels.Gateway{managed, selfHosted}, nil
		},
		EnvironmentMappingExistsFunc: func(string, string) (bool, error) { return true, nil },
		GetByUUIDFunc: func(id string) (*gwmodels.Gateway, error) {
			if id == selfHosted.UUID.String() {
				return selfHosted, nil
			}
			return managed, nil
		},
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) {
			return []gwmodels.GatewayEnvironmentMapping{{EnvironmentUUID: env}}, nil
		},
	}

	t.Run("no gateway named resolves to the managed gateway", func(t *testing.T) {
		got, err := resolveEgressGatewayForEnvironment(repo, "ou-1", env, nil)
		require.NoError(t, err)
		require.NoError(t, validateEgressPlacement(repo, got, nil))
		assert.Equal(t, managed.UUID, got.UUID)
	})

	t.Run("naming the self-hosted gateway places there", func(t *testing.T) {
		named := selfHosted.UUID.String()
		got, err := resolveEgressGatewayForEnvironment(repo, "ou-1", env, &named)
		require.NoError(t, err)
		require.NoError(t, validateEgressPlacement(repo, got, nil))
		assert.Equal(t, selfHosted.UUID, got.UUID)
	})

	t.Run("an artifact anchored to the self-hosted gateway keeps it", func(t *testing.T) {
		anchors := []string{selfHosted.UUID.String()}
		got, err := resolveEgressGatewayForArtifact(repo, "ou-1", env, anchors, nil)
		require.NoError(t, err)
		require.NoError(t, validateEgressPlacement(repo, got, anchors))
		assert.Equal(t, selfHosted.UUID, got.UUID)
	})

	t.Run("naming another gateway for an anchored artifact is refused", func(t *testing.T) {
		named := managed.UUID.String()
		_, err := resolveEgressGatewayForArtifact(repo, "ou-1", env, []string{selfHosted.UUID.String()}, &named)
		assert.ErrorIs(t, err, errPlacementFixed)
	})

	record := reconcile.Record{
		ArtifactRef:     gwmodels.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New()},
		EnvironmentUUID: env,
	}
	unplaced := &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) { return nil, nil },
	}

	t.Run("an unplaced LLM proxy and provider target the managed gateway", func(t *testing.T) {
		proxy := goldenProxy()
		kind := &LLMProxyKind{
			gatewayRepo: repo, deploymentRepo: unplaced,
			load: func(context.Context, gwmodels.ArtifactRef) (*gwmodels.LLMProxy, *gwmodels.LLMProvider, error) {
				return proxy, goldenProvider(), nil
			},
		}

		got, ok, err := kind.Targets(context.Background(), record)

		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, managed.UUID, got)
	})

	t.Run("an unplaced MCP proxy targets the managed gateway", func(t *testing.T) {
		kind := &MCPProxyKind{gatewayRepo: repo, deploymentRepo: unplaced, load: func(context.Context, gwmodels.ArtifactRef) (*mcpArtifact, error) {
			return &mcpArtifact{deployable: &gwmodels.MCPProxy{Configuration: gwmodels.MCPProxyConfig{
				Upstream: gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{URL: "https://93.184.216.34/mcp"}},
			}}}, nil
		}}

		got, ok, err := kind.Targets(context.Background(), record)

		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, managed.UUID, got)
	})
}
