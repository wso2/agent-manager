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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// clearGatewayManifestCache resets the process-wide backend to a fresh in-memory
// cache, undoing both any Set() from the test and any SetGatewayManifestCacheBackend
// swap another test may have left behind.
func clearGatewayManifestCache(t *testing.T) {
	t.Helper()
	original := gatewayManifestCache
	t.Cleanup(func() { gatewayManifestCache = original })
	gatewayManifestCache = NewInMemoryGatewayManifestCache()
}

// TestSaveGatewayPolicyManifestCachesWithoutWritingRow is the point of moving manifests
// out of the jsonb column: a push must land in the cache and must not update the row.
func TestSaveGatewayPolicyManifestCachesWithoutWritingRow(t *testing.T) {
	clearGatewayManifestCache(t)

	gateway := newGateway(t, models.GatewayRoleBoth, true)
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayID string) (*models.Gateway, error) {
			require.Equal(t, gateway.UUID.String(), gatewayID)
			return gateway, nil
		},
		// UpdateGatewayFunc is left nil: calling it panics, which is the assertion that
		// the push no longer writes the row.
	}
	svc := NewPlatformGatewayService(repo, nil)

	manifest := policyManifest("mcp-auth", "v1")
	require.NoError(t, svc.SaveGatewayPolicyManifest(context.Background(), gateway.UUID.String(), manifest))

	cached, ok, err := gatewayManifestCache.Get(context.Background(), gateway.UUID.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, manifest, cached)
	require.Equal(t, manifest, gatewayManifest(gateway))
}

// TestGatewayManifestIsPerGateway: a push describes only the gateway that sent it. Any
// other gateway, in this org or another, keeps answering with its own manifest.
func TestGatewayManifestIsPerGateway(t *testing.T) {
	clearGatewayManifestCache(t)
	ctx := context.Background()

	pusher := newGateway(t, models.GatewayRoleEgress, true)
	other := newGateway(t, models.GatewayRoleEgress, true)
	other.Manifest = policyManifest("row-policy", "v1")

	require.NoError(t, gatewayManifestCache.Set(ctx, pusher.UUID.String(), policyManifest("pushed-policy", "v1")))

	require.Equal(t, policyManifest("pushed-policy", "v1"), gatewayManifest(pusher))
	require.Equal(t, other.Manifest, gatewayManifest(other))
}

// TestGatewayManifestPushDoesNotLeakAcrossOrgs is the multi-tenant guarantee: a gateway
// in one org (self-hosted, so its manifest is whatever its operator runs) must not change
// the policies another org is offered or the Agent Identity check on its gateways.
func TestGatewayManifestPushDoesNotLeakAcrossOrgs(t *testing.T) {
	clearGatewayManifestCache(t)
	ctx := context.Background()

	foreign := newGateway(t, models.GatewayRoleEgress, true)
	foreign.OUID = "org-a"
	foreign.Hosting = models.GatewayHostingSelfHosted
	ours := newGateway(t, models.GatewayRoleEgress, true)
	ours.OUID = "org-b"

	require.NoError(t, gatewayManifestCache.Set(ctx, ours.UUID.String(), policyManifest("word-count-guardrail", "v1")))
	require.NoError(t, gatewayManifestCache.Set(ctx, foreign.UUID.String(),
		policyManifest("mcp-auth", "v1", "mcp-authz", "v1", "injected-policy", "v1")))

	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			require.Equal(t, "org-b", opts.OrganizationID)
			return []*models.Gateway{ours}, nil
		},
	}

	llm, err := intersectActiveGatewayLLMPolicies(repo, "org-b")
	require.NoError(t, err)
	require.Len(t, llm, 1)
	require.Contains(t, llm, "word-count-guardrail\x00v1")

	mcp, err := (&MCPProxyService{gatewayRepo: repo}).ListAvailableMCPPolicies(ctx, "org-b")
	require.NoError(t, err)
	require.Len(t, mcp.List, 1)
	require.Equal(t, "word-count-guardrail", mcp.List[0].Name)

	require.False(t, gatewayHasMCPIdentityPolicies(ours))
	require.True(t, gatewayHasMCPIdentityPolicies(foreign))
}

// TestSaveGatewayPolicyManifestUnknownGateway keeps the endpoint's 404 behaviour: an
// unknown gateway must not seed the cache.
func TestSaveGatewayPolicyManifestUnknownGateway(t *testing.T) {
	clearGatewayManifestCache(t)

	const unknownID = "11111111-1111-1111-1111-111111111111"
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) {
			//nolint:nilnil // GetByUUID reports "no such gateway" as (nil, nil).
			return nil, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	err := svc.SaveGatewayPolicyManifest(context.Background(), unknownID, map[string]interface{}{})
	require.ErrorIs(t, err, utils.ErrGatewayNotFound)

	_, ok, getErr := gatewayManifestCache.Get(context.Background(), unknownID)
	require.NoError(t, getErr)
	require.False(t, ok)
}

// TestDeleteGatewayDropsItsCachedManifest: a deleted gateway's manifest must not outlive
// it, and other gateways' entries are untouched.
func TestDeleteGatewayDropsItsCachedManifest(t *testing.T) {
	clearGatewayManifestCache(t)
	ctx := context.Background()

	gateway := newGateway(t, models.GatewayRoleEgress, true)
	survivor := newGateway(t, models.GatewayRoleEgress, true)
	require.NoError(t, gatewayManifestCache.Set(ctx, gateway.UUID.String(), policyManifest("a", "v1")))
	require.NoError(t, gatewayManifestCache.Set(ctx, survivor.UUID.String(), policyManifest("b", "v1")))

	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc:             func(string) (*models.Gateway, error) { return gateway, nil },
		HasGatewayDeploymentsFunc: func(string, string) (bool, error) { return false, nil },
		DeleteFunc:                func(string, string) error { return nil },
	}
	svc := NewPlatformGatewayService(repo, nil)

	require.NoError(t, svc.DeleteGateway(gateway.UUID.String(), gateway.OUID))

	_, ok, err := gatewayManifestCache.Get(ctx, gateway.UUID.String())
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = gatewayManifestCache.Get(ctx, survivor.UUID.String())
	require.NoError(t, err)
	require.True(t, ok)
}

// TestGatewayManifestFallsBackToRow covers the cold window after a restart: until the
// gateway pushes again, the manifest still on its row is what readers evaluate.
func TestGatewayManifestFallsBackToRow(t *testing.T) {
	clearGatewayManifestCache(t)

	gateway := newGateway(t, models.GatewayRoleBoth, true)
	gateway.Manifest = map[string]interface{}{"policies": []interface{}{}}

	require.Equal(t, gateway.Manifest, gatewayManifest(gateway))
	require.Nil(t, gatewayManifest(nil))
}

// TestInMemoryGatewayManifestCacheKeepsLatestPerGateway: a second push from the same
// gateway replaces its first; entries of different gateways are independent.
func TestInMemoryGatewayManifestCacheKeepsLatestPerGateway(t *testing.T) {
	ctx := context.Background()
	cache := NewInMemoryGatewayManifestCache()
	first := policyManifest("a", "v1")
	second := policyManifest("b", "v1")
	other := policyManifest("c", "v1")

	require.NoError(t, cache.Set(ctx, "gw-1", first))
	require.NoError(t, cache.Set(ctx, "gw-2", other))
	require.NoError(t, cache.Set(ctx, "gw-1", second))

	cached, ok, err := cache.Get(ctx, "gw-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, second, cached)

	require.NoError(t, cache.Delete(ctx, "gw-1"))
	_, ok, err = cache.Get(ctx, "gw-1")
	require.NoError(t, err)
	require.False(t, ok)

	cached, ok, err = cache.Get(ctx, "gw-2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, other, cached)
}

// TestGatewayManifest_CacheReadErrorFallsBackToRow covers a Redis-unreachable-style
// failure: gatewayManifest must degrade to the row rather than propagate the error,
// since manifests are advisory.
func TestGatewayManifest_CacheReadErrorFallsBackToRow(t *testing.T) {
	clearGatewayManifestCache(t)
	gatewayManifestCache = &failingManifestCacheBackend{}

	gateway := newGateway(t, models.GatewayRoleBoth, true)
	gateway.Manifest = map[string]interface{}{"policies": []interface{}{}}

	require.Equal(t, gateway.Manifest, gatewayManifest(gateway))
}

// TestDeleteGatewaySucceedsWhenManifestCacheFails: the cache is advisory, so failing to
// drop an entry must not fail a delete that already committed.
func TestDeleteGatewaySucceedsWhenManifestCacheFails(t *testing.T) {
	clearGatewayManifestCache(t)
	gatewayManifestCache = &failingManifestCacheBackend{}

	gateway := newGateway(t, models.GatewayRoleEgress, true)
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc:             func(string) (*models.Gateway, error) { return gateway, nil },
		HasGatewayDeploymentsFunc: func(string, string) (bool, error) { return false, nil },
		DeleteFunc:                func(string, string) error { return nil },
	}

	require.NoError(t, NewPlatformGatewayService(repo, nil).DeleteGateway(gateway.UUID.String(), gateway.OUID))
}

// policyManifest builds a manifest in the shape gateways push, from name/version pairs.
func policyManifest(nameVersionPairs ...string) map[string]interface{} {
	items := make([]interface{}, 0, len(nameVersionPairs)/2)
	for i := 0; i+1 < len(nameVersionPairs); i += 2 {
		items = append(items, map[string]interface{}{"name": nameVersionPairs[i], "version": nameVersionPairs[i+1]})
	}
	return map[string]interface{}{"policies": items}
}

// failingManifestCacheBackend simulates an unreachable external cache backend.
type failingManifestCacheBackend struct{}

func (f *failingManifestCacheBackend) Set(context.Context, string, map[string]interface{}) error {
	return errCacheUnavailable
}

func (f *failingManifestCacheBackend) Get(context.Context, string) (map[string]interface{}, bool, error) {
	return nil, false, errCacheUnavailable
}

func (f *failingManifestCacheBackend) Delete(context.Context, string) error {
	return errCacheUnavailable
}

var errCacheUnavailable = errors.New("cache backend unavailable")
