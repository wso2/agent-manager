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
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1/identitymocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// stubInfraManager overrides only ListOrgEnvironments; any other call panics
// via the nil embedded interface, which is exactly what a unit test wants.
// scopeTestProxy builds an endpoint-era proxy: capabilities live on the endpoint's
// MCPEndpointConfig (migration 032), not on the parent proxy config.
func scopeTestProxy(handle string, tools ...string) *gwmodels.MCPProxy {
	endpoint := gwmodels.MCPProxyEndpoint{UUID: uuid.New(), Handle: "primary"}
	if len(tools) > 0 {
		toolMaps := make([]map[string]interface{}, 0, len(tools))
		for _, tl := range tools {
			toolMaps = append(toolMaps, map[string]interface{}{"name": tl})
		}
		endpoint.Configuration = gwmodels.MCPEndpointConfig{
			Capabilities: &gwmodels.MCPProxyCapabilities{Tools: &toolMaps},
		}
	}
	return &gwmodels.MCPProxy{
		UUID:      uuid.New(),
		Artifact:  &gwmodels.Artifact{Handle: handle},
		Endpoints: []gwmodels.MCPProxyEndpoint{endpoint},
	}
}

func newScopeSvcForTest(scopeRepo gwrepo.MCPProxyScopeRepository, proxy *gwmodels.MCPProxy) MCPProxyScopeService {
	// Tests that focus on the create path don't all set GetFunc; provide a
	// default "not found" so the duplicate-check path can run.
	if mock, ok := scopeRepo.(*gwrepomocks.MCPProxyScopeRepositoryMock); ok && mock.GetFunc == nil {
		mock.GetFunc = func(_ context.Context, _ uuid.UUID, _ string) (*gwmodels.MCPProxyScope, error) {
			return nil, gorm.ErrRecordNotFound
		}
	}

	proxyRepo := &gwrepomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(ctx context.Context, handle, orgUUID string) (*gwmodels.MCPProxy, error) {
			if proxy != nil && proxy.Artifact.Handle == handle {
				return proxy, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	return NewMCPProxyScopeService(scopeRepo, proxyRepo, nil, nil, noopRedeployer{}, slog.Default())
}

// newScopeSvcForTestWithRedeployer is newScopeSvcForTest with an injectable
// MCPProxyRedeployer, for tests asserting on re-emission behavior.
func newScopeSvcForTestWithRedeployer(scopeRepo gwrepo.MCPProxyScopeRepository, proxy *gwmodels.MCPProxy, redeployer MCPProxyRedeployer) MCPProxyScopeService {
	if mock, ok := scopeRepo.(*gwrepomocks.MCPProxyScopeRepositoryMock); ok && mock.GetFunc == nil {
		mock.GetFunc = func(_ context.Context, _ uuid.UUID, _ string) (*gwmodels.MCPProxyScope, error) {
			return nil, gorm.ErrRecordNotFound
		}
	}
	proxyRepo := &gwrepomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(ctx context.Context, handle, orgUUID string) (*gwmodels.MCPProxy, error) {
			if proxy != nil && proxy.Artifact.Handle == handle {
				return proxy, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	return NewMCPProxyScopeService(scopeRepo, proxyRepo, nil, nil, redeployer, slog.Default())
}

// noopRedeployer is the default MCPProxyRedeployer stub for tests that don't
// exercise re-emission behavior directly.
type noopRedeployer struct{}

func (noopRedeployer) RedeployMCPProxy(context.Context, *gwmodels.MCPProxy, string) error { return nil }

func (noopRedeployer) EnsureResourceServersForProxy(context.Context, string, *gwmodels.MCPProxy, []string) {
}

// redeployCall records one RedeployMCPProxy invocation for assertions.
type redeployCall struct {
	proxy *gwmodels.MCPProxy
	ouID  string
}

// ensureRSCall records one EnsureResourceServersForProxy invocation for assertions.
type ensureRSCall struct {
	ouID    string
	proxy   *gwmodels.MCPProxy
	actions []string
}

// recordingRedeployer records every RedeployMCPProxy/EnsureResourceServersForProxy
// call so tests can assert re-emission and resource-server-ensure happened.
// The service now launches EnsureResourceServersForProxy from a detached
// goroutine, so ensureCalls is mutex-guarded; read it via ensureCallsSnapshot
// (polled with require.Eventually), never directly.
type recordingRedeployer struct {
	calls []redeployCall
	err   error

	mu          sync.Mutex
	ensureCalls []ensureRSCall
}

func (r *recordingRedeployer) RedeployMCPProxy(_ context.Context, proxy *gwmodels.MCPProxy, ouID string) error {
	r.calls = append(r.calls, redeployCall{proxy: proxy, ouID: ouID})
	return r.err
}

func (r *recordingRedeployer) EnsureResourceServersForProxy(_ context.Context, ouID string, proxy *gwmodels.MCPProxy, actions []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureCalls = append(r.ensureCalls, ensureRSCall{ouID: ouID, proxy: proxy, actions: actions})
}

// ensureCallsSnapshot safely copies the calls recorded so far.
func (r *recordingRedeployer) ensureCallsSnapshot() []ensureRSCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ensureRSCall(nil), r.ensureCalls...)
}

func TestMCPProxyScopeCreate_ValidatesAction(t *testing.T) {
	svc := newScopeSvcForTest(&gwrepomocks.MCPProxyScopeRepositoryMock{}, scopeTestProxy("gh-proxy", "list_repos"))
	for _, action := range []string{"", "has space", "with:colon", "with/slash", strings.Repeat("a", 101)} {
		_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
			gwmodels.MCPProxyScopeInput{Action: action, Tools: []string{"list_repos"}})
		assert.ErrorIs(t, err, httpx.ErrInvalidInput, "action %q", action)
	}
}

func TestMCPProxyScopeCreate_StrictToolValidationWhenCapabilitiesKnown(t *testing.T) {
	svc := newScopeSvcForTest(&gwrepomocks.MCPProxyScopeRepositoryMock{}, scopeTestProxy("gh-proxy", "list_repos", "get_repo"))
	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read", Tools: []string{"list_repos", "not_a_tool"}})
	assert.ErrorIs(t, err, httpx.ErrInvalidInput)
	assert.Contains(t, err.Error(), "not_a_tool")
}

func TestMCPProxyScopeCreate_PermissiveWhenNoCapabilitiesStored(t *testing.T) {
	var created *gwmodels.MCPProxyScope
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		CreateFunc: func(ctx context.Context, s *gwmodels.MCPProxyScope) error { created = s; return nil },
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy")) // zero tools stored
	res, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read", Tools: []string{"anything_goes"}})
	assert.NoError(t, err)
	assert.Equal(t, "gh-proxy", res.ProxyHandle)
	assert.Equal(t, []string{"anything_goes"}, created.Tools)
}

func TestMCPProxyScopeCreate_DuplicateActionConflicts(t *testing.T) {
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(ctx context.Context, proxyUUID uuid.UUID, action string) (*gwmodels.MCPProxyScope, error) {
			return &gwmodels.MCPProxyScope{Action: action}, nil // already exists
		},
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy", "list_repos"))
	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read", Tools: []string{"list_repos"}})
	assert.ErrorIs(t, err, httpx.ErrConflict)
}

func TestMCPProxyScopeCreate_UnknownProxy404(t *testing.T) {
	svc := newScopeSvcForTest(&gwrepomocks.MCPProxyScopeRepositoryMock{}, nil)
	_, err := svc.Create(context.Background(), "org-uuid", "org", "ghost",
		gwmodels.MCPProxyScopeInput{Action: "read", Tools: []string{"t"}})
	assert.ErrorIs(t, err, gwerr.ErrMCPProxyNotFound)
}

func TestMCPProxyScopeDelete_MissingIsNotFound(t *testing.T) {
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		DeleteFunc: func(ctx context.Context, proxyUUID uuid.UUID, action string) error { return gorm.ErrRecordNotFound },
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy"))
	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy", "read")
	assert.ErrorIs(t, err, gwerr.ErrScopeNotFound)
}

func TestMCPProxyScopeCreate_TriggersReEmit(t *testing.T) {
	proxy := scopeTestProxy("gh-proxy", "list_repos")
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		CreateFunc: func(ctx context.Context, s *gwmodels.MCPProxyScope) error { return nil },
	}
	redeployer := &recordingRedeployer{}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read", Tools: []string{"list_repos"}})

	assert.NoError(t, err)
	if assert.Len(t, redeployer.calls, 1) {
		assert.Same(t, proxy, redeployer.calls[0].proxy)
		assert.Equal(t, "org-uuid", redeployer.calls[0].ouID)
	}
}

func TestMCPProxyScopeCreate_ReEmitFailureIsReturned(t *testing.T) {
	proxy := scopeTestProxy("gh-proxy", "list_repos")
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		CreateFunc: func(ctx context.Context, s *gwmodels.MCPProxyScope) error { return nil },
	}
	redeployer := &recordingRedeployer{err: errors.New("deploy boom")}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read", Tools: []string{"list_repos"}})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deploy boom")
}

func TestMCPProxyScopeUpdate_TriggersReEmit(t *testing.T) {
	proxy := scopeTestProxy("gh-proxy", "list_repos")
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, _ uuid.UUID, action string) (*gwmodels.MCPProxyScope, error) {
			return &gwmodels.MCPProxyScope{Action: action, Tools: []string{"list_repos"}}, nil
		},
		UpdateFunc: func(ctx context.Context, s *gwmodels.MCPProxyScope) error { return nil },
	}
	redeployer := &recordingRedeployer{}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	desc := "updated"
	_, err := svc.Update(context.Background(), "org-uuid", "org", "gh-proxy", "read",
		gwmodels.MCPProxyScopeUpdateInput{Description: &desc})

	assert.NoError(t, err)
	assert.Len(t, redeployer.calls, 1)
}

// identityEnabledEndpoint builds an endpoint with identity security enabled or
// disabled, bound to one environment.
func identityEnabledEndpoint(handle string, envUUID, artifactUUID uuid.UUID, enabled bool) gwmodels.MCPProxyEndpoint {
	ep := gwmodels.MCPProxyEndpoint{
		UUID:   uuid.New(),
		Handle: handle,
		Environments: []gwmodels.MCPProxyEndpointEnvironment{
			{EnvironmentUUID: envUUID, ArtifactUUID: artifactUUID},
		},
	}
	if enabled {
		on := true
		ep.Configuration = gwmodels.MCPEndpointConfig{
			Security: &gwmodels.SecurityConfig{Enabled: &on, Identity: &gwmodels.IdentitySecurity{Enabled: &on}},
		}
	}
	return ep
}

func TestMCPProxyScopeDelete_CleansThunderBestEffort(t *testing.T) {
	envA, envB := uuid.New(), uuid.New()
	proxy := &gwmodels.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &gwmodels.Artifact{Handle: "gh-proxy"},
		Endpoints: []gwmodels.MCPProxyEndpoint{
			identityEnabledEndpoint("a", envA, uuid.New(), true),
			identityEnabledEndpoint("b", envB, uuid.New(), false),
		},
	}
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		DeleteFunc: func(ctx context.Context, proxyUUID uuid.UUID, action string) error { return nil },
	}
	proxyRepo := &gwrepomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(ctx context.Context, handle, orgUUID string) (*gwmodels.MCPProxy, error) { return proxy, nil },
	}
	infra := &gwtest.Core{ListEnvironmentsFunc: func(ctx context.Context, ouID string) ([]*modkit.Environment, error) {
		return []*modkit.Environment{
			{Name: "env-a", ID: envA.String()},
			{Name: "env-b", ID: envB.String()},
		}, nil
	}}

	type removedPermission struct {
		roleID string
		req    identityv1.RolePermission
	}
	var removed []removedPermission
	envAClient := &identitymocks.ClientMock{
		RemoveResourceServerActionFunc: func(ctx context.Context, proxyHandle, action string) (string, error) {
			assert.Equal(t, "gh-proxy", proxyHandle)
			assert.Equal(t, "read", action)
			return "rs-1", nil
		},
		ListRolesFunc: func(ctx context.Context, ouID string, offset, limit int) ([]identityv1.Role, int, error) {
			assert.Equal(t, "", ouID, "role sweep must list every role in the env-Thunder, not filter by a platform OU")
			if offset > 0 {
				return nil, 1, nil
			}
			return []identityv1.Role{
				{ID: "role-1", Permissions: []identityv1.RolePermission{
					{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read", "gh-proxy:write"}},
				}},
			}, 1, nil
		},
		RemoveRolePermissionsFunc: func(ctx context.Context, roleID string, req identityv1.RolePermission) error {
			removed = append(removed, removedPermission{roleID: roleID, req: req})
			return nil
		},
	}
	resolver := &identitymocks.DirectoryMock{
		ForFunc: func(ctx context.Context, ouID, envName string) (identityv1.Client, error) {
			if envName == "env-a" {
				return envAClient, nil
			}
			// cleanupDeletedScope does not gate on the endpoint's identity flag, so it
			// resolves env-b too; env-b has identity disabled, so env-Thunder is
			// unavailable there. Best-effort cleanup must warn and continue past this.
			return nil, errors.New("env-thunder unavailable for identity-disabled env")
		},
	}
	redeployer := &recordingRedeployer{}
	svc := NewMCPProxyScopeService(scopeRepo, proxyRepo, infra, resolver, redeployer, slog.Default())

	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy", "read")

	assert.NoError(t, err)
	if assert.Len(t, removed, 1) {
		assert.Equal(t, "role-1", removed[0].roleID)
		assert.Equal(t, identityv1.RolePermission{
			ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read"},
		}, removed[0].req)
	}
	assert.Len(t, redeployer.calls, 1)
}

func TestMCPProxyScopeDelete_BestEffortSurvivesResolverError(t *testing.T) {
	envA := uuid.New()
	proxy := &gwmodels.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &gwmodels.Artifact{Handle: "gh-proxy"},
		Endpoints: []gwmodels.MCPProxyEndpoint{
			identityEnabledEndpoint("a", envA, uuid.New(), true),
		},
	}
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		DeleteFunc: func(ctx context.Context, proxyUUID uuid.UUID, action string) error { return nil },
	}
	proxyRepo := &gwrepomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(ctx context.Context, handle, orgUUID string) (*gwmodels.MCPProxy, error) { return proxy, nil },
	}
	infra := &gwtest.Core{ListEnvironmentsFunc: func(ctx context.Context, ouID string) ([]*modkit.Environment, error) {
		return []*modkit.Environment{{Name: "env-a", ID: envA.String()}}, nil
	}}
	resolver := &identitymocks.DirectoryMock{
		ForFunc: func(ctx context.Context, ouID, envName string) (identityv1.Client, error) {
			return nil, errors.New("env-thunder unreachable")
		},
	}
	redeployer := &recordingRedeployer{}
	svc := NewMCPProxyScopeService(scopeRepo, proxyRepo, infra, resolver, redeployer, slog.Default())

	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy", "read")

	assert.NoError(t, err, "Thunder cleanup is best-effort and must never fail the delete")
	assert.Len(t, redeployer.calls, 1, "redeploy must still run after a best-effort cleanup failure")
}

func TestListEnvironmentScopes_IncludesUndeployedIdentityProxies(t *testing.T) {
	envUUID := uuid.MustParse("3fa85f64-5717-4562-b3fc-2c963f66afa6")
	on := true
	identityEndpoint := func(artifact uuid.UUID, enabled bool) gwmodels.MCPProxyEndpoint {
		ep := gwmodels.MCPProxyEndpoint{
			UUID:   uuid.New(),
			Handle: "primary",
			Environments: []gwmodels.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envUUID, ArtifactUUID: artifact},
			},
		}
		if enabled {
			ep.Configuration = gwmodels.MCPEndpointConfig{
				Security: &gwmodels.SecurityConfig{Enabled: &on, Identity: &gwmodels.IdentitySecurity{Enabled: &on}},
			}
		}
		return ep
	}
	// deployed and undeployed are both identity-enabled; deployment state must no
	// longer affect the catalog. off has identity disabled and must be excluded.
	deployed := &gwmodels.MCPProxy{
		UUID: uuid.New(), Artifact: &gwmodels.Artifact{Handle: "gh-proxy", Name: "GitHub"},
		Endpoints: []gwmodels.MCPProxyEndpoint{identityEndpoint(uuid.New(), true)},
	}
	off := &gwmodels.MCPProxy{
		UUID: uuid.New(), Artifact: &gwmodels.Artifact{Handle: "plain", Name: "Plain"},
		Endpoints: []gwmodels.MCPProxyEndpoint{identityEndpoint(uuid.New(), false)},
	}
	undeployed := &gwmodels.MCPProxy{
		UUID: uuid.New(), Artifact: &gwmodels.Artifact{Handle: "idle", Name: "Idle"},
		Endpoints: []gwmodels.MCPProxyEndpoint{identityEndpoint(uuid.New(), true)},
	}

	proxyRepo := &gwrepomocks.MCPProxyRepositoryMock{
		ListFunc: func(_ context.Context, _ string, f gwrepo.MCPProxyListFilter) ([]*gwmodels.MCPProxy, int64, error) {
			if f.Offset > 0 {
				return nil, 0, nil
			}
			return []*gwmodels.MCPProxy{deployed, off, undeployed}, 0, nil
		},
	}
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		ListByProxyUUIDsFunc: func(ctx context.Context, ids []uuid.UUID) ([]gwmodels.MCPProxyScope, error) {
			assert.ElementsMatch(t, []uuid.UUID{deployed.UUID, undeployed.UUID}, ids)
			return []gwmodels.MCPProxyScope{
				{MCPProxyUUID: deployed.UUID, Action: "read", Description: "d"},
				{MCPProxyUUID: undeployed.UUID, Action: "read", Description: "d"},
			}, nil
		},
	}
	infra := &gwtest.Core{ListEnvironmentsFunc: func(ctx context.Context, ouID string) ([]*modkit.Environment, error) {
		return []*modkit.Environment{{Name: "dev", ID: envUUID.String()}}, nil
	}}
	svc := NewMCPProxyScopeService(scopeRepo, proxyRepo, infra, nil, nil, slog.Default())
	entries, err := svc.ListEnvironmentScopes(context.Background(), "org-uuid", "dev")
	assert.NoError(t, err)
	assert.Equal(t, []gwmodels.EnvironmentScopeEntry{
		{Scope: "gh-proxy:read", Description: "d", MCPProxyID: "gh-proxy", MCPProxyName: "GitHub"},
		{Scope: "idle:read", Description: "d", MCPProxyID: "idle", MCPProxyName: "Idle"},
	}, entries)
}

// A scope with no tools is a declared-but-unbound catalog entry: still grantable
// to a role (and still projected into Thunder on that grant), but enforced on no
// tool. Clearing the last tool must therefore persist as an empty list rather
// than being rejected — see validateScopeTools.
func TestMCPProxyScopeUpdate_ClearingAllToolsIsAllowed(t *testing.T) {
	proxy := scopeTestProxy("gh-proxy", "list_repos")
	var updated *gwmodels.MCPProxyScope
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, _ uuid.UUID, action string) (*gwmodels.MCPProxyScope, error) {
			return &gwmodels.MCPProxyScope{Action: action, Tools: []string{"list_repos"}}, nil
		},
		UpdateFunc: func(_ context.Context, s *gwmodels.MCPProxyScope) error { updated = s; return nil },
	}
	redeployer := &recordingRedeployer{}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	res, err := svc.Update(context.Background(), "org-uuid", "org", "gh-proxy", "read",
		gwmodels.MCPProxyScopeUpdateInput{Tools: []string{}})

	assert.NoError(t, err)
	// Empty, not nil: the jsonb column must hold [] rather than null.
	assert.NotNil(t, updated.Tools)
	assert.Empty(t, updated.Tools)
	assert.Empty(t, res.Scope.Tools)
	assert.Len(t, redeployer.calls, 1, "clearing tools must still re-emit the gateway policies")
}

// nil Tools means "leave unchanged" (gwmodels.MCPProxyScopeUpdateInput), so a
// description-only PUT must not clear the tool list now that empty is legal.
func TestMCPProxyScopeUpdate_OmittedToolsLeavesToolsUnchanged(t *testing.T) {
	var updated *gwmodels.MCPProxyScope
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, _ uuid.UUID, action string) (*gwmodels.MCPProxyScope, error) {
			return &gwmodels.MCPProxyScope{Action: action, Tools: []string{"list_repos"}}, nil
		},
		UpdateFunc: func(_ context.Context, s *gwmodels.MCPProxyScope) error { updated = s; return nil },
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy", "list_repos"))

	desc := "updated"
	_, err := svc.Update(context.Background(), "org-uuid", "org", "gh-proxy", "read",
		gwmodels.MCPProxyScopeUpdateInput{Description: &desc})

	assert.NoError(t, err)
	assert.Equal(t, []string{"list_repos"}, updated.Tools)
}

// A scope can be declared before any tool is wired to it.
func TestMCPProxyScopeCreate_AllowsScopeWithNoTools(t *testing.T) {
	var created *gwmodels.MCPProxyScope
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		CreateFunc: func(_ context.Context, s *gwmodels.MCPProxyScope) error { created = s; return nil },
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy", "list_repos"))

	res, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read"})

	assert.NoError(t, err)
	assert.NotNil(t, created.Tools)
	assert.Empty(t, created.Tools)
	assert.Equal(t, "read", res.Scope.Action)
}

// TestMCPProxyScopeCreate_DelegatesResourceServerEnsureWithFullActionSet guards
// the improvement this test file was extended for: saving a scope must
// delegate to MCPProxyService.EnsureResourceServersForProxy with the proxy's
// FULL current action set (not just the one just saved) — the per-environment
// identity/deployment gating is MCPProxyService's own concern, tested at that
// level (see mcp_rs_identifier_test.go).
func TestMCPProxyScopeCreate_DelegatesResourceServerEnsureWithFullActionSet(t *testing.T) {
	proxy := &gwmodels.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &gwmodels.Artifact{Handle: "gh-proxy"},
		Endpoints: []gwmodels.MCPProxyEndpoint{
			identityEnabledEndpoint("a", uuid.New(), uuid.New(), true),
		},
	}
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, _ uuid.UUID, _ string) (*gwmodels.MCPProxyScope, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateFunc: func(_ context.Context, s *gwmodels.MCPProxyScope) error { return nil },
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]gwmodels.MCPProxyScope, error) {
			return []gwmodels.MCPProxyScope{{Action: "list_repos"}, {Action: "read"}}, nil
		},
	}
	redeployer := &recordingRedeployer{}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read"})

	assert.NoError(t, err)
	// The ensure call runs on a detached goroutine (see mcpProxyScopeService.Create),
	// so wait for it rather than asserting on ensureCalls the instant Create returns.
	require.Eventually(t, func() bool { return len(redeployer.ensureCallsSnapshot()) == 1 },
		time.Second, 10*time.Millisecond, "expected exactly one ensure call")
	call := redeployer.ensureCallsSnapshot()[0]
	assert.Equal(t, "org-uuid", call.ouID)
	assert.Same(t, proxy, call.proxy)
	assert.ElementsMatch(t, []string{"list_repos", "read"}, call.actions,
		"must pass the proxy's full current action set, not just the one just saved")
}

// TestMCPProxyScopeCreate_EnsureListFailureDoesNotFailSave guards the
// best-effort contract at this layer: a failure loading the proxy's scopes
// for the ensure call must never fail the scope save itself (it already
// committed).
func TestMCPProxyScopeCreate_EnsureListFailureDoesNotFailSave(t *testing.T) {
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, _ uuid.UUID, _ string) (*gwmodels.MCPProxyScope, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateFunc: func(_ context.Context, s *gwmodels.MCPProxyScope) error { return nil },
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]gwmodels.MCPProxyScope, error) {
			return nil, errors.New("db unavailable")
		},
	}
	proxy := &gwmodels.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &gwmodels.Artifact{Handle: "gh-proxy"},
		Endpoints: []gwmodels.MCPProxyEndpoint{
			identityEnabledEndpoint("a", uuid.New(), uuid.New(), true),
		},
	}
	redeployer := &recordingRedeployer{}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	res, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		gwmodels.MCPProxyScopeInput{Action: "read"})

	assert.NoError(t, err, "a scope-list failure for the ensure call must not fail the scope save")
	assert.Equal(t, "read", res.Scope.Action)
	assert.Empty(t, redeployer.ensureCallsSnapshot())
	assert.Len(t, redeployer.calls, 1, "the save must still re-emit gateway policy despite the ensure failure")
}

// TestMCPProxyScopeUpdate_DelegatesResourceServerEnsure is Create's sibling for
// Update: editing an existing scope must also (re)trigger the ensure, in case
// it never registered yet (e.g. the scope predates this improvement, or the
// proxy's environment binding was only just created — see
// MCPProxyService.EnsureResourceServersForProxy's doc comment).
func TestMCPProxyScopeUpdate_DelegatesResourceServerEnsure(t *testing.T) {
	proxy := &gwmodels.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &gwmodels.Artifact{Handle: "gh-proxy"},
		Endpoints: []gwmodels.MCPProxyEndpoint{
			identityEnabledEndpoint("a", uuid.New(), uuid.New(), true),
		},
	}
	scopeRepo := &gwrepomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, _ uuid.UUID, action string) (*gwmodels.MCPProxyScope, error) {
			return &gwmodels.MCPProxyScope{Action: action}, nil
		},
		UpdateFunc: func(_ context.Context, _ *gwmodels.MCPProxyScope) error { return nil },
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]gwmodels.MCPProxyScope, error) {
			return []gwmodels.MCPProxyScope{{Action: "read"}}, nil
		},
	}
	redeployer := &recordingRedeployer{}
	svc := newScopeSvcForTestWithRedeployer(scopeRepo, proxy, redeployer)

	desc := "updated"
	_, err := svc.Update(context.Background(), "org-uuid", "org", "gh-proxy", "read",
		gwmodels.MCPProxyScopeUpdateInput{Description: &desc})

	assert.NoError(t, err)
	require.Eventually(t, func() bool { return len(redeployer.ensureCallsSnapshot()) == 1 },
		time.Second, 10*time.Millisecond, "expected exactly one ensure call")
	calls := redeployer.ensureCallsSnapshot()
	assert.Same(t, proxy, calls[0].proxy)
	assert.Equal(t, []string{"read"}, calls[0].actions)
}
