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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The endpoint's own path is the MCP path when it declares one; /mcp is only the fallback for an endpoint with no path.
func TestMCPUpstreamURL(t *testing.T) {
	tests := map[string]string{
		"http://svc:8080":                        "http://svc:8080/mcp",
		"http://svc:8080/":                       "http://svc:8080/mcp",
		"http://svc:8080/mcp":                    "http://svc:8080/mcp",
		"http://svc:8080/mcp/":                   "http://svc:8080/mcp",
		"http://svc:8080/api/v1/mcp-server":      "http://svc:8080/api/v1/mcp-server",
		"http://svc.ns.svc.cluster.local/v1/mcp": "http://svc.ns.svc.cluster.local/v1/mcp",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, mcpUpstreamURL(in))
		})
	}
}

const (
	bindingProject     = "p1"
	bindingProjectUUID = "p1-uuid"
	bindingHandle      = "weather-mcp"
	bindingEnv         = "development"
	bindingServiceURL  = "http://weather.dp-p1-development.svc.cluster.local:8080"
)

// bindingFixture runs the real binding service over in-memory repositories and a reconcile core
// whose MCP kind renders the fixture's own bindings with the production render code.
type bindingFixture struct {
	svc     *ProjectMCPProxyService
	proxy   *models.MCPProxy
	envUUID uuid.UUID
	gateway *models.Gateway // nil: the environment has no gateway

	serviceURL    string
	serviceURLErr error
	// releaseBindingNames are the release binding names the upstream lookups were given, in order.
	releaseBindingNames []string

	// status rows by artifact UUID; StartDeploy writes a DEPLOYING one.
	status map[uuid.UUID]*models.DeploymentStatusRecord
	// deployed holds every deployment StartDeploy was asked to send, in order.
	deployed []*models.Deployment
	// others are artifacts already deployed on the gateway: deployment id -> content.
	others map[uuid.UUID][]byte

	endpointWrites int
	hub            *stubEventHub

	// gatewayLocks are the gateways the binding transaction locked, in order; onGatewayLock runs while the lock
	// is taken, standing in for another binding that committed just before it.
	gatewayLocks  []string
	onGatewayLock func()

	// proxyUpdates counts proxy row writes; rerenders counts redeploys queued for the proxy's bindings.
	proxyUpdates int
	rerenders    int
	// rerendered is the artifact of each redeploy queued, in order.
	rerendered []uuid.UUID
}

func newBindingFixture(t *testing.T) *bindingFixture {
	t.Helper()
	ctxPath := "/weather"
	project, projectUUID := bindingProject, bindingProjectUUID
	proxyUUID := uuid.New()
	f := &bindingFixture{
		envUUID:    uuid.New(),
		serviceURL: bindingServiceURL,
		status:     map[uuid.UUID]*models.DeploymentStatusRecord{},
		others:     map[uuid.UUID][]byte{},
		hub:        &stubEventHub{},
		proxy: &models.MCPProxy{
			UUID:        proxyUUID,
			Status:      models.StatusCreated,
			ProjectUUID: &projectUUID,
			ProjectName: &project,
			Configuration: models.MCPProxyConfig{
				Name: "Weather", Version: "v1", Context: &ctxPath,
				Source: &models.MCPProxySource{ComponentName: "weather", EndpointName: "mcp"},
			},
			// A project proxy has no endpoint until its first binding.
			Artifact: &models.Artifact{UUID: proxyUUID, Handle: bindingHandle, Name: "p1 Weather", Version: "v1"},
		},
	}
	f.gateway = newGateway(t, models.GatewayRoleBoth, true)
	f.gateway.Vhost = "dev-gw.example.com"

	copyProxy := func() *models.MCPProxy {
		cp := *f.proxy
		cp.Endpoints = append([]models.MCPProxyEndpoint(nil), f.proxy.Endpoints...)
		for i := range cp.Endpoints {
			cp.Endpoints[i].Environments = append([]models.MCPProxyEndpointEnvironment(nil), f.proxy.Endpoints[i].Environments...)
		}
		return &cp
	}
	repo := &repomocks.MCPProxyRepositoryMock{
		GetInProjectFunc: func(_ context.Context, handle, _, projectUUID string) (*models.MCPProxy, error) {
			if handle != bindingHandle || projectUUID != bindingProjectUUID {
				return nil, utils.ErrMCPProxyNotFound
			}
			return copyProxy(), nil
		},
		GetInProjectByNameFunc: func(_ context.Context, handle, _, projectName string) (*models.MCPProxy, error) {
			if handle != bindingHandle || projectName != bindingProject {
				return nil, utils.ErrMCPProxyNotFound
			}
			return copyProxy(), nil
		},
		UpdateFunc: func(_ context.Context, _ *gorm.DB, p *models.MCPProxy, _ string) error {
			f.proxyUpdates++
			f.proxy.Configuration = p.Configuration
			f.proxy.Description = p.Description
			return nil
		},
		GetByHandleForUpdateFunc: func(_ context.Context, _ *gorm.DB, handle, _ string) (*models.MCPProxy, error) {
			if handle != bindingHandle {
				return nil, gorm.ErrRecordNotFound
			}
			return copyProxy(), nil
		},
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		CreateEndpointFunc: func(_ context.Context, _ *gorm.DB, e *models.MCPProxyEndpoint) error {
			f.endpointWrites++
			e.UpdatedAt = time.Now()
			f.proxy.Endpoints = append(f.proxy.Endpoints, *e)
			return nil
		},
		UpdateEndpointFunc: func(_ context.Context, _ *gorm.DB, e *models.MCPProxyEndpoint) error {
			f.endpointWrites++
			for i := range f.proxy.Endpoints {
				if f.proxy.Endpoints[i].UUID == e.UUID {
					// As the repository does, the stored capability discovery state is kept.
					cfg := e.Configuration
					cfg.CapabilityDiscovery = f.proxy.Endpoints[i].Configuration.CapabilityDiscovery
					f.proxy.Endpoints[i].Configuration = cfg
					return nil
				}
			}
			return gorm.ErrRecordNotFound
		},
		ClearIdentityCleanupIDFunc: func(_ context.Context, endpointUUID uuid.UUID, cleanupID string) error {
			// As the repository does, only while the stored ID is still cleanupID.
			for i := range f.proxy.Endpoints {
				if f.proxy.Endpoints[i].UUID == endpointUUID && f.proxy.Endpoints[i].Configuration.IdentityCleanupID == cleanupID {
					f.proxy.Endpoints[i].Configuration.IdentityCleanupID = ""
				}
			}
			return nil
		},
		MarkIdentityCleanupIDFunc: func(_ context.Context, endpointUUID uuid.UUID, cleanupID string) (bool, error) {
			// As the repository does, only while off identity with no ID set.
			for i := range f.proxy.Endpoints {
				cfg := &f.proxy.Endpoints[i].Configuration
				if f.proxy.Endpoints[i].UUID == endpointUUID && !mcpIdentityEnabled(cfg.Security) && cfg.IdentityCleanupID == "" {
					cfg.IdentityCleanupID = cleanupID
					return true, nil
				}
			}
			return false, nil
		},
		AddEndpointEnvironmentFunc: func(_ context.Context, _ *gorm.DB, ee *models.MCPProxyEndpointEnvironment) error {
			for i := range f.proxy.Endpoints {
				if f.proxy.Endpoints[i].UUID == ee.EndpointUUID {
					f.proxy.Endpoints[i].Environments = append(f.proxy.Endpoints[i].Environments, *ee)
					return nil
				}
			}
			return gorm.ErrRecordNotFound
		},
	}
	artifactRepo := &repomocks.ArtifactRepositoryMock{
		CreateFunc: func(*gorm.DB, *models.Artifact) error { return nil },
	}
	deployments := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, artifactUUID uuid.UUID) ([]string, error) {
			if row, ok := f.status[artifactUUID]; ok && row.StatusDesired == models.DeploymentStatusDeployed {
				return []string{row.GatewayUUID.String()}, nil
			}
			return nil, nil
		},
		StatusRecordsFunc: func(_ context.Context, _ string, artifactUUID uuid.UUID) ([]models.DeploymentStatusRecord, error) {
			if row, ok := f.status[artifactUUID]; ok {
				return []models.DeploymentStatusRecord{*row}, nil
			}
			return nil, nil
		},
		LockStatusRowsFunc: func(context.Context, *gorm.DB, string, []uuid.UUID) error { return nil },
		QueueRenderFunc: func(_ context.Context, _ *gorm.DB, _ string, artifactUUID uuid.UUID, _ []uuid.UUID, _ string, _ repositories.QueueScope) (int64, error) {
			f.rerenders++
			f.rerendered = append(f.rerendered, artifactUUID)
			return 0, nil
		},
		LockEnvironmentMappingFunc: mappedEverywhere,
		StartDeployFunc: func(ctx context.Context, tx *gorm.DB, p models.Placement, d *models.Deployment) error {
			if err := startDeployStub(ctx, tx, p, d); err != nil {
				return err
			}
			id := uuid.New()
			d.DeploymentID, d.ArtifactUUID = id, p.ArtifactUUID
			f.deployed = append(f.deployed, d)
			f.status[p.ArtifactUUID] = &models.DeploymentStatusRecord{
				ArtifactUUID: p.ArtifactUUID, GatewayUUID: p.GatewayUUID, DeploymentID: &id,
				Status: models.DeploymentStatusDeploying, StatusDesired: models.DeploymentStatusDeployed,
			}
			return nil
		},
		ListForGatewaySyncFunc: func(_ context.Context, _, _ string) ([]models.GatewaySyncRow, error) {
			var rows []models.GatewaySyncRow
			for id := range f.others {
				rows = append(rows, models.GatewaySyncRow{
					ArtifactRef:  models.ArtifactRef{ArtifactUUID: id},
					DeploymentID: id.String(), State: models.GatewaySyncStateDeployed,
				})
			}
			for artifactUUID, row := range f.status {
				if row.DeploymentID != nil {
					rows = append(rows, models.GatewaySyncRow{
						ArtifactRef:  models.ArtifactRef{ArtifactUUID: artifactUUID},
						DeploymentID: row.DeploymentID.String(), State: models.GatewaySyncStateDeployed,
					})
				}
			}
			return rows, nil
		},
		ListContentForGatewayFunc: func(_ context.Context, _, _ string, ids []uuid.UUID) ([]models.GatewayDeploymentContent, error) {
			var out []models.GatewayDeploymentContent
			for _, id := range ids {
				if content, ok := f.others[id]; ok {
					out = append(out, models.GatewayDeploymentContent{
						ArtifactRef: models.ArtifactRef{ArtifactUUID: id}, DeploymentID: id, Content: content,
					})
				}
				for _, d := range f.deployed {
					if d.DeploymentID == id {
						out = append(out, models.GatewayDeploymentContent{
							ArtifactRef: models.ArtifactRef{ArtifactUUID: d.ArtifactUUID}, DeploymentID: id, Content: d.Content,
						})
					}
				}
			}
			return out, nil
		},
	}
	deployments.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return deployments }

	gatewayRepo := gatewayFixtureRepo(t, f.envUUID.String(), nil)
	gatewayRepo.ListWithFiltersFunc = func(flt repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		var gateways []*models.Gateway
		if f.gateway != nil && flt.EnvironmentID != nil && *flt.EnvironmentID == f.envUUID.String() {
			gateways = append(gateways, f.gateway)
		}
		return gateways, nil
	}
	gatewayRepo.GetByUUIDFunc = func(id string) (*models.Gateway, error) {
		if f.gateway != nil && f.gateway.UUID.String() == id {
			return f.gateway, nil
		}
		return nil, gorm.ErrRecordNotFound
	}
	gatewayRepo.EnvironmentMappingExistsFunc = func(gatewayID, environmentID string) (bool, error) {
		return f.gateway != nil && f.gateway.UUID.String() == gatewayID && environmentID == f.envUUID.String(), nil
	}
	gatewayRepo.AcquireGatewayLockFunc = func(_ *gorm.DB, gatewayID string) error {
		f.gatewayLocks = append(f.gatewayLocks, gatewayID)
		if f.onGatewayLock != nil {
			f.onGatewayLock()
		}
		return nil
	}

	base := &MCPProxyService{
		repo:           repo,
		endpointRepo:   endpointRepo,
		deploymentRepo: deployments,
		gatewayRepo:    gatewayRepo,
		artifactRepo:   artifactRepo,
		logger:         discardLogger(),
		encryptionKey:  []byte("0123456789abcdef0123456789abcdef"),
		tx:             sentinelTx,
	}
	base.kind = &MCPProxyKind{
		gatewayRepo: gatewayRepo, deploymentRepo: deployments,
		load: func(_ context.Context, ref models.ArtifactRef) (*mcpArtifact, error) {
			for i := range f.proxy.Endpoints {
				endpoint := &f.proxy.Endpoints[i]
				for j := range endpoint.Environments {
					if endpoint.Environments[j].ArtifactUUID == ref.ArtifactUUID {
						return newMCPArtifact(f.proxy, endpoint, &endpoint.Environments[j], nil), nil
					}
				}
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	base.core = reconcile.NewCore(nil, deployments, f.hub, reconcile.Kinds{base.kind}, reconcile.Config{}, discardLogger()).WithGateways(gatewayRepo)

	oc := &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, projectName, name string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Name: name, ProjectName: projectName}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			if name != bindingEnv {
				return nil, utils.ErrNotFound
			}
			return &models.EnvironmentResponse{UUID: f.envUUID.String(), Name: name}, nil
		},
		GetReleaseBindingEndpointServiceURLFunc: func(_ context.Context, _, projectName, component, endpoint, env, bindingName string) (string, error) {
			f.releaseBindingNames = append(f.releaseBindingNames, bindingName)
			assert.Equal(t, bindingProject, projectName, "the release binding lookup is always project-scoped")
			assert.Equal(t, "weather", component)
			assert.Equal(t, "mcp", endpoint)
			assert.Equal(t, bindingEnv, env)
			return f.serviceURL, f.serviceURLErr
		},
	}
	f.svc = NewProjectMCPProxyService(base, oc, nil)
	return f
}

func (f *bindingFixture) put(t *testing.T, req models.MCPProxyEnvironmentRequest) (*models.MCPProxyEnvironmentResponse, bool, error) {
	t.Helper()
	return f.svc.PutEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv, req)
}

// binding is the stored (endpoint, environment) binding, or nils.
func (f *bindingFixture) binding() (*models.MCPProxyEndpoint, *models.MCPProxyEndpointEnvironment) {
	return resolveMCPEndpointForEnv(f.proxy, f.envUUID.String())
}

func deployedYAML(t *testing.T, d *models.Deployment) MCPProxyDeploymentYAML {
	t.Helper()
	var out MCPProxyDeploymentYAML
	require.NoError(t, yaml.Unmarshal(d.Content, &out))
	return out
}

func TestPutEnv_ComponentNotDeployed_409UpstreamNotReady_NothingStored(t *testing.T) {
	for name, outcome := range map[string]struct {
		url string
		err error
	}{
		"not reconciled":     {url: ""},
		"no release binding": {err: occlient.ErrReleaseBindingNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			f.serviceURL, f.serviceURLErr = outcome.url, outcome.err
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
			require.ErrorIs(t, err, utils.ErrMCPUpstreamNotReady)
			assert.Zero(t, f.endpointWrites, "nothing is stored")
			assert.Empty(t, f.deployed)
		})
	}
}

func TestPutEnv_UnknownEndpoint_409UpstreamNotReady(t *testing.T) {
	f := newBindingFixture(t)
	f.serviceURLErr = occlient.ErrEndpointNotFound
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.ErrorIs(t, err, utils.ErrMCPUpstreamNotReady)
	assert.Zero(t, f.endpointWrites)

	f.serviceURLErr = errors.New("openchoreo unavailable")
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrMCPUpstreamNotReady, "a lookup failure is not masked as not-ready")
	assert.NotErrorIs(t, err, utils.ErrInvalidInput)
}

func TestPutEnv_NoGateway_StoredWithReasonNoGateway(t *testing.T) {
	f := newBindingFixture(t)
	f.gateway = nil

	resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Empty(t, resp.DeploymentStatus, "no gateway: no deployment status")
	assert.Equal(t, "NO_GATEWAY", resp.DeploymentStatusReason)
	assert.Empty(t, resp.GatewayID)
	assert.Empty(t, resp.PublicURL)
	assert.Equal(t, bindingEnv, resp.EnvironmentName)
	assert.Equal(t, f.envUUID.String(), resp.EnvironmentUUID)
	assert.Empty(t, f.deployed, "nothing is sent")

	require.Len(t, f.proxy.Endpoints, 1, "the binding is its own endpoint")
	endpoint, ee := f.binding()
	require.NotNil(t, ee, "the binding is stored")
	assert.Equal(t, bindingEnv, endpoint.Handle)
	assert.Equal(t, bindingEnv, endpoint.Name)
	require.NotNil(t, endpoint.Configuration.Upstream)
	assert.Equal(t, bindingServiceURL+"/mcp", endpoint.Configuration.Upstream.URL, "an endpoint with no path gets /mcp on the resolved URL")
	require.NotNil(t, endpoint.Configuration.Security)
	require.NotNil(t, endpoint.Configuration.Security.APIKey)
	assert.Equal(t, "X-API-Key", endpoint.Configuration.Security.APIKey.Key, "default security is an X-API-Key header")
	assert.Equal(t, "header", endpoint.Configuration.Security.APIKey.In)

	// Identical re-PUT with still no gateway writes nothing.
	writes := f.endpointWrites
	_, created, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, writes, f.endpointWrites)
}

func TestPutEnv_ContextClash_409ContextInUse(t *testing.T) {
	for name, other := range map[string]string{
		"equal":    "/p1/weather",
		"contains": "/p1",
		"nested":   "/p1/weather/tools",
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			f.others[uuid.New()] = []byte("apiVersion: gateway.api-platform.wso2.com/v1\nkind: RestApi\nspec:\n  context: " + other + "\n")
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
			require.ErrorIs(t, err, utils.ErrMCPContextInUse)
			assert.Zero(t, f.endpointWrites, "nothing is stored")
			assert.Empty(t, f.deployed)
		})
	}

	t.Run("a sibling path is no clash", func(t *testing.T) {
		f := newBindingFixture(t)
		f.others[uuid.New()] = []byte("spec:\n  context: /p1/weather-v2\n")
		_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
		require.NoError(t, err)
	})
}

func TestPutEnv_ContextClashCommittedBeforeGatewayLock_409ContextInUse(t *testing.T) {
	f := newBindingFixture(t)
	// Another binding with an overlapping context commits after this PUT's first check, while it waits for the
	// gateway lock.
	f.onGatewayLock = func() {
		f.others[uuid.New()] = []byte("spec:\n  context: /p1/weather\n")
	}
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.ErrorIs(t, err, utils.ErrMCPContextInUse)
	require.NotEmpty(t, f.gatewayLocks, "the binding transaction locks its gateway")
	for _, locked := range f.gatewayLocks {
		assert.Equal(t, f.gateway.UUID.String(), locked)
	}
	assert.Empty(t, f.deployed, "nothing is sent")
}

func TestPutEnv_NoGateway_TakesNoGatewayLock(t *testing.T) {
	f := newBindingFixture(t)
	f.gateway = nil
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.Empty(t, f.gatewayLocks)
}

func TestPutEnv_ContextPrefixedOnGatewayReadBackUnprefixed(t *testing.T) {
	f := newBindingFixture(t)
	resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.True(t, created)

	require.Len(t, f.deployed, 1)
	artifact := deployedYAML(t, f.deployed[0])
	assert.Equal(t, "/p1/weather", artifact.Spec.Context, "the gateway serves the project-prefixed context")
	assert.Equal(t, "p1 Weather", artifact.Spec.DisplayName)
	assert.Equal(t, bindingServiceURL+"/mcp", artifact.Spec.Upstream.URL)

	assert.Equal(t, "/weather", resp.Context, "context reads back as IP sent it")
	assert.Equal(t, "https://dev-gw.example.com/p1/weather/mcp", resp.PublicURL)
	assert.Equal(t, models.MCPDeploymentStatusDeploying, resp.DeploymentStatus)
	assert.Equal(t, f.gateway.UUID.String(), resp.GatewayID)
	require.Len(t, f.hub.published, 1)
	assert.Equal(t, f.gateway.UUID.String(), f.hub.published[0].GatewayID)
}

func TestPutEnv_IdenticalOnDeployed_NoRedeploy(t *testing.T) {
	for _, status := range []models.DeploymentStatus{models.DeploymentStatusDeployed, models.DeploymentStatusDeploying} {
		t.Run(string(status), func(t *testing.T) {
			f := newBindingFixture(t)
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
			require.NoError(t, err)
			_, ee := f.binding()
			f.status[ee.ArtifactUUID].Status = status
			writes := f.endpointWrites

			resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
			require.NoError(t, err)
			assert.False(t, created)
			assert.Len(t, f.deployed, 1, "an identical PUT makes no new deployment")
			assert.Len(t, f.hub.published, 1)
			assert.Equal(t, writes, f.endpointWrites, "nor writes the binding")
			assert.Equal(t, models.MCPDeploymentStatusOf(models.UserStatus{Status: status}), resp.DeploymentStatus)
		})
	}

	t.Run("a changed input redeploys", func(t *testing.T) {
		f := newBindingFixture(t)
		_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
		require.NoError(t, err)
		_, ee := f.binding()
		f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed

		f.serviceURL = "http://weather-v2.dp-p1-development.svc.cluster.local:8080"
		_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
		require.NoError(t, err)
		require.Len(t, f.deployed, 2)
		assert.Equal(t, "http://weather-v2.dp-p1-development.svc.cluster.local:8080/mcp", deployedYAML(t, f.deployed[1]).Spec.Upstream.URL)
	})
}

// A component deployed on several tracks has a release binding per track in the environment: without a name the PUT
// is refused, not resolved to whichever binding OpenChoreo lists first.
func TestPutEnv_SeveralReleaseBindings_400NothingStored(t *testing.T) {
	for name, lookupErr := range map[string]error{
		"no name, several bindings":            occlient.ErrAmbiguousReleaseBinding,
		"named binding of another environment": occlient.ErrReleaseBindingMismatch,
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			f.serviceURLErr = lookupErr
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.NotErrorIs(t, err, utils.ErrMCPUpstreamNotReady)
			assert.Zero(t, f.endpointWrites, "nothing is stored")
			assert.Empty(t, f.deployed)
		})
	}
}

func TestPutEnv_ReleaseBindingNamePassedStoredAndReadBack(t *testing.T) {
	f := newBindingFixture(t)
	name := " weather-beta-development "
	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{ReleaseBindingName: &name})
	require.NoError(t, err)
	assert.Equal(t, []string{"weather-beta-development"}, f.releaseBindingNames, "the trimmed name picks the binding")
	assert.Equal(t, "weather-beta-development", resp.ReleaseBindingName)
	endpoint, _ := f.binding()
	assert.Equal(t, "weather-beta-development", endpoint.Configuration.ReleaseBindingName)

	got, err := f.svc.GetEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	assert.Equal(t, "weather-beta-development", got.ReleaseBindingName)
}

func TestPutEnv_ChangedReleaseBindingName_Redeploys(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed

	// Same upstream URL, as two tracks' services can share one: the name alone makes the binding differ.
	name := "weather-beta-development"
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{ReleaseBindingName: &name})
	require.NoError(t, err)
	assert.Len(t, f.deployed, 2)
}

// A newcomer that came to overlap a live binding never makes the binding's identical ensure-PUT fail: the no-op
// returns before the context check.
func TestPutEnv_IdenticalOnDeployed_SkipsTheContextCheck(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	// Deployed rows the gateway holds, refreshed so the check would see an overlap if it ran.
	f.others[uuid.New()] = []byte("spec:\n  context: /p1/weather/tools\n")
	f.deployed[0].Content = []byte("spec:\n  context: /p1/elsewhere\n")

	resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, models.MCPDeploymentStatusDeployed, resp.DeploymentStatus)
	assert.Len(t, f.deployed, 1, "nothing is redeployed")
}

func TestPutEnv_IdenticalOnFailed_Requeues(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusFailed
	f.status[ee.ArtifactUUID].StatusReason = "UPSTREAM_UNREACHABLE"
	writes := f.endpointWrites

	resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Len(t, f.deployed, 2, "a PUT on a Failed binding re-queues it")
	assert.Equal(t, writes, f.endpointWrites, "without rewriting the unchanged binding")
	assert.Equal(t, models.MCPDeploymentStatusDeploying, resp.DeploymentStatus)
}

func TestPutEnv_JWT_400(t *testing.T) {
	enabled := true
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{
		Enabled: &enabled, JWT: &models.MCPProxyJWTSecurity{Enabled: &enabled, Issuers: []string{"ThunderKeyManager"}},
	}})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "jwt security is not available yet")
	assert.Zero(t, f.endpointWrites)
}

// thunderCalls records the identity service calls the binding made.
type thunderCalls struct {
	ensured []struct{ env, handle, identifier string }
	// deleted records each resource server delete; identifier is the one it was found by, empty for a delete by
	// bare handle.
	deleted []struct{ env, handle, identifier string }
	// swept counts the role listings a delete made to drop the proxy's scopes from roles.
	swept int
	// deleteErr, when set, fails every resource server delete after recording it.
	deleteErr error
	// onDelete, when set, runs inside each resource server delete, as a concurrent write landing meanwhile would.
	onDelete func()
	// onEnsure, when set, runs inside each resource server registration, as a concurrent write landing meanwhile would.
	onEnsure func()
	// listRolesErr, when set, fails the role listing a delete sweeps grants with.
	listRolesErr error
}

// forget drops what was recorded so far, such as the leftover removal a new binding starts with.
func (c *thunderCalls) forget() {
	c.ensured, c.deleted, c.swept = nil, nil, 0
}

// withThunder gives the fixture an identity service that records resource server writes.
func (f *bindingFixture) withThunder() *thunderCalls {
	calls := &thunderCalls{}
	f.svc.base.resolver = &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, _, _, envName string) (thundersvc.EnvIdentityClient, error) {
			return &clientmocks.EnvIdentityClientMock{
				EnsureProxyResourceServerFunc: func(_ context.Context, handle, _, identifier string, _ []string) (string, error) {
					calls.ensured = append(calls.ensured, struct{ env, handle, identifier string }{envName, handle, identifier})
					if calls.onEnsure != nil {
						calls.onEnsure()
					}
					return "rs-1", nil
				},
				DeleteProxyResourceServerFunc: func(_ context.Context, handle string) error {
					calls.deleted = append(calls.deleted, struct{ env, handle, identifier string }{envName, handle, ""})
					return calls.deleteErr
				},
				DeleteProxyResourceServerByIdentifierFunc: func(_ context.Context, handle, identifier string) error {
					calls.deleted = append(calls.deleted, struct{ env, handle, identifier string }{envName, handle, identifier})
					if calls.onDelete != nil {
						calls.onDelete()
					}
					return calls.deleteErr
				},
				ListRolesFunc: func(context.Context, string, int, int) ([]thundersvc.ThunderRole, int, error) {
					calls.swept++
					return nil, 0, calls.listRolesErr
				},
			}, nil
		},
	}
	return calls
}

func identityBinding() *models.MCPProxyEnvironmentSecurity {
	enabled := true
	return &models.MCPProxyEnvironmentSecurity{Enabled: &enabled, Identity: &models.IdentitySecurity{Enabled: &enabled}}
}

// An identity binding is stored without an api key, renders mcp-auth against ThunderKeyManager instead of the api-key
// policy, and is registered as a resource server whose identifier is the binding's public URL, so a token requested
// for that URL is accepted.
func TestPutEnv_Identity_RendersMCPAuthAndRegistersTheResourceServer(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()

	resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.True(t, created)

	require.NotNil(t, resp.Security)
	require.NotNil(t, resp.Security.Identity)
	assert.True(t, isBoolTrue(resp.Security.Identity.Enabled))
	assert.Nil(t, resp.Security.APIKey)

	endpoint, ee := f.binding()
	assert.True(t, mcpIdentityEnabled(endpoint.Configuration.Security))
	assert.Nil(t, endpoint.Configuration.Security.APIKey, "identity carries no api key")
	assert.False(t, mcpProxyAPIKeySecurityEnabled(f.proxy, ee.EnvironmentUUID.String()), "agents get no api key")

	require.Len(t, f.deployed, 1)
	var auth *models.MCPPolicy
	for _, policy := range deployedYAML(t, f.deployed[0]).Spec.Policies {
		assert.NotEqual(t, apiKeyAuthPolicyName, policy.Name)
		if policy.Name == mcpAuthPolicyName {
			auth = &policy
		}
	}
	require.NotNil(t, auth, "the binding validates OAuth tokens")
	assert.Equal(t, []interface{}{mcpIdentityIssuerKeyManager}, auth.Params["issuers"])

	require.Len(t, calls.ensured, 1)
	assert.Equal(t, bindingEnv, calls.ensured[0].env)
	assert.Equal(t, bindingHandle, calls.ensured[0].handle)
	assert.Equal(t, resp.PublicURL, calls.ensured[0].identifier, "the resource identifier is the binding's public URL")
	assert.Contains(t, calls.ensured[0].identifier, "/"+bindingProject+"/weather", "it carries the project path")

	// An identical PUT repairs the registration again, without redeploying.
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	_, created, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Len(t, f.deployed, 1)
	assert.Len(t, calls.ensured, 2)
}

func TestPutEnv_IdentityWithAPIKey_400(t *testing.T) {
	enabled := true
	f := newBindingFixture(t)
	security := identityBinding()
	security.APIKey = &models.APIKeySecurity{Enabled: &enabled}
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: security})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "mutually exclusive")
	assert.Zero(t, f.endpointWrites)
}

// An apiKey given without enabled is on, as it is without identity, so with identity it is refused, not dropped.
func TestPutEnv_IdentityWithAPIKeyEnabledOmitted_400(t *testing.T) {
	f := newBindingFixture(t)
	security := identityBinding()
	security.APIKey = &models.APIKeySecurity{Key: "X-API-Key"}
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: security})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "mutually exclusive")
	assert.Zero(t, f.endpointWrites)
}

// A binding moved off identity no longer accepts its tokens, so its resource server is removed; one that stays on
// identity keeps it.
func TestPutEnv_LeavingIdentity_DeletesTheResourceServer(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()

	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	require.Len(t, calls.deleted, 1)
	assert.Equal(t, bindingEnv, calls.deleted[0].env)
	assert.Equal(t, bindingHandle, calls.deleted[0].handle)
	assert.Equal(t, resp.PublicURL, calls.deleted[0].identifier, "the resource server registered at the binding's URL")
	endpoint, _ := f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID, "a removal that succeeded leaves nothing pending")
}

// Removing the resource server when a binding leaves identity is best effort. The write marks the binding pending,
// an identical PUT retries the removal only while it is, and a removal that succeeds clears the mark, so a grant
// made afterwards is not swept by later identical PUTs.
func TestPutEnv_IdenticalPUTRetriesOnlyAPendingIdentityCleanup(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	f.svc.base.mcpProxyScopeRepo = &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}}, nil
		},
	}

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()
	calls.deleteErr = errors.New("identity service unavailable")
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err, "the cleanup is best effort")
	require.Len(t, calls.deleted, 1)
	endpoint, ee := f.binding()
	assert.NotEmpty(t, endpoint.Configuration.IdentityCleanupID, "the failed removal stays pending")

	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	calls.deleteErr = nil
	_, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.False(t, created)
	require.Len(t, calls.deleted, 2, "the identical PUT retries the pending removal")
	assert.Equal(t, 1, calls.swept, "and drops the scope from roles, which the failed removal never reached")
	endpoint, _ = f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID)

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.Len(t, calls.deleted, 2, "nothing pending: a grant made since is left alone")
	assert.Len(t, f.deployed, 2, "identical PUTs never redeploy")
}

// A binding back on identity keeps a pending removal's ID until that removal succeeds: while it fails the binding is
// not registered again, so its old grants do not apply, and an identical PUT retries the removal, then registers.
func TestPutEnv_BackToIdentityKeepsAFailingCleanupPending(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	require.Len(t, calls.ensured, 1)
	calls.forget()
	calls.deleteErr = errors.New("identity service unavailable")
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	endpoint, ee := f.binding()
	assert.NotEmpty(t, endpoint.Configuration.IdentityCleanupID, "the write back to identity keeps the pending removal")
	assert.Empty(t, calls.ensured, "not registered again while the old resource server's removal fails")

	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	calls.deleteErr = nil
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	endpoint, _ = f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID)
	assert.Len(t, calls.ensured, 1, "the identical PUT removed the old one, then registered")
}

// A removal whose cleanup ID cannot be cleared is not done: the binding does not register, since the ID left behind
// would have the next identical PUT remove that registration; the next PUT repeats the removal, clears, then registers.
func TestSyncBindingIdentity_UnclearedCleanupIDHoldsTheRegistration(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	endpointRepo := f.svc.base.endpointRepo.(*repomocks.MCPProxyEndpointRepositoryMock)
	clearID := endpointRepo.ClearIdentityCleanupIDFunc
	endpointRepo.ClearIdentityCleanupIDFunc = func(context.Context, uuid.UUID, string) error { return errors.New("db down") }

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.Len(t, calls.deleted, 1, "the new binding's leftover removal ran")
	assert.Empty(t, calls.ensured, "but with its ID not cleared the binding does not register")
	endpoint, ee := f.binding()
	assert.NotEmpty(t, endpoint.Configuration.IdentityCleanupID)

	endpointRepo.ClearIdentityCleanupIDFunc = clearID
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.Len(t, calls.deleted, 2)
	assert.Len(t, calls.ensured, 1)
	endpoint, _ = f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID)
}

// A registration can finish after a newer write took the binding off identity and that write's own removal already
// ran. Nothing else would remove what it registered, so the sync marks the binding for removal and removes it.
func TestSyncBindingIdentity_AStaleRegistrationIsRemovedAgain(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	calls.forget()

	// While this identical PUT registers, a newer PUT takes the binding off identity, its removal done and ID cleared.
	calls.onEnsure = func() {
		calls.onEnsure = nil
		apiKey := true
		for i := range f.proxy.Endpoints {
			f.proxy.Endpoints[i].Configuration.Security = &models.SecurityConfig{
				Enabled: &apiKey, APIKey: &models.APIKeySecurity{Enabled: &apiKey, Key: "X-API-Key", In: "header"},
			}
			f.proxy.Endpoints[i].Configuration.IdentityCleanupID = ""
		}
	}
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	require.Len(t, calls.ensured, 1, "the stale registration ran")
	require.Len(t, calls.deleted, 1, "and was removed again")
	endpoint, _ := f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID, "its removal is done")
}

// A removal whose role sweep fails is not done: roles may still grant the proxy's scopes, so the cleanup ID stays
// and the next identical PUT sweeps again.
func TestBindingIdentityCleanup_AFailedRoleSweepStaysPending(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	f.svc.base.mcpProxyScopeRepo = &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}}, nil
		},
	}
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()

	calls.listRolesErr = errors.New("identity service unavailable")
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, calls.swept)
	endpoint, ee := f.binding()
	assert.NotEmpty(t, endpoint.Configuration.IdentityCleanupID, "the sweep failed, so the removal stays pending")

	calls.listRolesErr = nil
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.Equal(t, 2, calls.swept, "the identical PUT sweeps again")
	endpoint, _ = f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID)
}

// A scope save registers the resource servers of the proxy's identity bindings, except one with a removal pending,
// which registers only once its own sync has removed the old resource server.
func TestEnsureResourceServersForProxy_SkipsABindingWithACleanupPending(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()
	f.svc.base.infraManager = &infraResourceManagerStubForScopeRefresh{
		ListOrgEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: bindingEnv}}, nil
		},
	}

	f.svc.base.EnsureResourceServersForProxy(context.Background(), testOrg, f.proxy, []string{"read"})
	assert.Len(t, calls.ensured, 1, "an identity binding with nothing pending registers")

	for i := range f.proxy.Endpoints {
		f.proxy.Endpoints[i].Configuration.IdentityCleanupID = "cleanup-1"
	}
	f.svc.base.EnsureResourceServersForProxy(context.Background(), testOrg, f.proxy, []string{"read"})
	assert.Len(t, calls.ensured, 1, "one with a removal pending does not")
}

// A stale removal can land after a newer write put the binding back on identity and registered it. The sync re-reads
// the binding after acting and applies its current state again, so the binding is registered once more rather than
// left without its resource server.
func TestSyncBindingIdentity_AStaleRemovalIsFollowedByTheNewerRegistration(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()

	// While this PUT's removal runs, a newer PUT commits the binding back on identity, its own sync done.
	calls.onDelete = func() {
		calls.onDelete = nil
		for i := range f.proxy.Endpoints {
			f.proxy.Endpoints[i].Configuration.Security = mcpIdentitySecurity()
			f.proxy.Endpoints[i].Configuration.IdentityCleanupID = ""
		}
	}
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	require.Len(t, calls.deleted, 1, "the stale removal ran")
	assert.Len(t, calls.ensured, 1, "and the binding, back on identity, was registered again after it")
}

// A removal that finishes after the binding left identity again clears only its own ID: the newer write's ID, whose
// removal is still to run, stays for a retry. Each write that leaves identity gets a new ID.
func TestBindingIdentityCleanup_AStaleRemovalKeepsTheNewerID(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.deleteErr = errors.New("identity service unavailable")
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	endpoint, _ := f.binding()
	first := endpoint.Configuration.IdentityCleanupID
	require.NotEmpty(t, first)

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	endpoint, _ = f.binding()
	second := endpoint.Configuration.IdentityCleanupID
	require.NotEmpty(t, second)
	assert.NotEqual(t, first, second, "leaving identity again starts a new cleanup")

	// The first removal, finishing late, succeeds but must not clear the second's ID.
	calls.deleteErr = nil
	f.svc.cleanUpBindingIdentity(context.Background(), testOrg, f.proxy, f.envUUID, bindingEnv, endpoint.UUID, first)
	endpoint, _ = f.binding()
	assert.Equal(t, second, endpoint.Configuration.IdentityCleanupID)
}

// A binding back on identity while a removal is still pending has the removal done first, so grants made for its old
// registration do not apply again, then registers afresh.
func TestPutEnv_BackToIdentityFinishesAPendingCleanupFirst(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()
	calls.deleteErr = errors.New("identity service unavailable")
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	calls.deleteErr = nil

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.Len(t, calls.deleted, 2, "the pending removal ran")
	assert.Len(t, calls.ensured, 1, "and the binding registered again")
	endpoint, _ := f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID)
}

// A new binding first removes any resource server an earlier, deleted binding of the proxy left in the environment
// when its removal failed, so that one's grants never apply to it; only then does it register. While the removal
// fails it does not register, and an identical PUT retries.
func TestPutEnv_NewBindingRemovesALeftoverResourceServerFirst(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	calls.deleteErr = errors.New("identity service unavailable")

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	require.Len(t, calls.deleted, 1, "the leftover removal ran")
	assert.Empty(t, calls.ensured, "not registered while a leftover may still carry old grants")
	endpoint, ee := f.binding()
	assert.NotEmpty(t, endpoint.Configuration.IdentityCleanupID)

	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	calls.deleteErr = nil
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.Len(t, calls.deleted, 2)
	assert.Len(t, calls.ensured, 1)
	endpoint, _ = f.binding()
	assert.Empty(t, endpoint.Configuration.IdentityCleanupID)
}

// A binding with no gateway has no public URL to register yet; the PUT after the gateway arrives registers it.
func TestPutEnv_Identity_NoGatewayRegistersOnceAGatewayIsThere(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	gateway := f.gateway
	f.gateway = nil

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	assert.Empty(t, calls.ensured, "nothing to register without a public URL")

	f.gateway = gateway
	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	require.Len(t, calls.ensured, 1)
	assert.Equal(t, resp.PublicURL, calls.ensured[0].identifier)
}

// An api-key binding is no resource server.
func TestPutEnv_APIKey_RegistersNoResourceServer(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.Empty(t, calls.ensured)
}

// A binding with a resource server to remove that cannot read its public URL, the resource server's identifier, is not
// deleted: once gone the URL could not be read again, so the delete returns the error and the binding stays for a retry.
func TestDeleteEnv_IdentifierLookupFailure_KeepsTheBinding(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	calls.forget()
	lookupErr := errors.New("db down")
	deployments := f.svc.base.deploymentRepo.(*repomocks.DeploymentRepositoryMock)
	deployments.PlacedGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) { return nil, lookupErr }
	repo := f.svc.base.repo.(*repomocks.MCPProxyRepositoryMock)
	locks := len(repo.GetByHandleForUpdateCalls())

	err = f.svc.DeleteEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, lookupErr)
	assert.Len(t, repo.GetByHandleForUpdateCalls(), locks, "nothing was deleted")
	_, still := f.binding()
	assert.NotNil(t, still)
	assert.Empty(t, calls.deleted)
}

// A binding never on identity has no resource server: deleting it makes no identity call and reads no scopes, so a
// proxy with OAuth in another environment does not page through roles on every delete here.
func TestDeleteEnv_NeverOnIdentity_TouchesNoIdentityService(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	lookups := 0
	f.svc.base.mcpProxyScopeRepo = &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) {
			lookups++
			return []models.MCPProxyScope{{Action: "read"}}, nil
		},
	}
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	calls.forget()
	lookups = 0
	// The fixture runs no delete transaction; the lock ends the delete there, past the point the scopes would be read.
	repo := f.svc.base.repo.(*repomocks.MCPProxyRepositoryMock)
	repo.GetByHandleForUpdateFunc = func(context.Context, *gorm.DB, string, string) (*models.MCPProxy, error) {
		return nil, gorm.ErrRecordNotFound
	}

	err = f.svc.DeleteEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	assert.Len(t, repo.GetByHandleForUpdateCalls(), 2, "the delete reached its lock")
	assert.Zero(t, lookups)
	assert.Empty(t, calls.deleted)
	assert.Zero(t, calls.swept)
}

// A failed scope lookup removes nothing, rather than deleting the resource server and leaving its grants in roles; a
// binding delete stops with the error, keeping the binding for a retry.
func TestBindingResourceServerCleanup_FailedScopeLookupRemovesNothing(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	lookupErr := errors.New("db down")
	f.svc.base.mcpProxyScopeRepo = &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) { return nil, lookupErr },
	}

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: identityBinding()})
	require.NoError(t, err)
	endpoint, _ := f.binding()
	endpointRepo := f.svc.base.endpointRepo.(*repomocks.MCPProxyEndpointRepositoryMock)
	f.svc.cleanUpBindingIdentity(context.Background(), testOrg, f.proxy, f.envUUID, bindingEnv, endpoint.UUID, "cleanup-1")
	assert.Empty(t, calls.deleted)
	assert.Empty(t, endpointRepo.ClearIdentityCleanupIDCalls(), "the ID stays for a retry")

	err = f.svc.DeleteEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, lookupErr)
	_, still := f.binding()
	assert.NotNil(t, still, "the binding is kept for a retry")
	assert.Empty(t, calls.deleted)
}

// The cleanup passes the proxy's scopes, as the proxy delete does, so the roles granting them are cleaned up.
func TestBindingIdentityCleanup_DropsTheProxyScopesFromRoles(t *testing.T) {
	f := newBindingFixture(t)
	calls := f.withThunder()
	f.svc.base.mcpProxyScopeRepo = &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}, {Action: "write"}}, nil
		},
	}

	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	endpoint, _ := f.binding()
	calls.forget()

	f.svc.cleanUpBindingIdentity(context.Background(), testOrg, f.proxy, f.envUUID, bindingEnv, endpoint.UUID, "")
	require.Len(t, calls.deleted, 1)
	assert.Equal(t, resp.PublicURL, calls.deleted[0].identifier,
		"found by the binding's public URL: the identity service returns no handle for a resource server")
	assert.Equal(t, 2, calls.swept, "one role sweep per scope")
}

// A body that leaves security enabled with no variant gets the default api key in X-API-Key, as an omitted security
// does: only an explicit enabled false turns security off, so a caller never ends up with an open endpoint.
func TestPutEnv_SecurityWithNoVariant_DefaultsToAPIKey(t *testing.T) {
	enabled := true
	for name, security := range map[string]*models.MCPProxyEnvironmentSecurity{
		"empty":           {},
		"enabled, no key": {Enabled: &enabled},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: security})
			require.NoError(t, err)

			endpoint, ee := f.binding()
			require.NotNil(t, endpoint.Configuration.Security)
			require.NotNil(t, endpoint.Configuration.Security.APIKey)
			assert.True(t, isBoolTrue(endpoint.Configuration.Security.Enabled))
			assert.Equal(t, "X-API-Key", endpoint.Configuration.Security.APIKey.Key)
			assert.Equal(t, "header", endpoint.Configuration.Security.APIKey.In)
			assert.True(t, mcpProxyAPIKeySecurityEnabled(f.proxy, ee.EnvironmentUUID.String()))

			require.Len(t, f.deployed, 1)
			var names []string
			for _, policy := range deployedYAML(t, f.deployed[0]).Spec.Policies {
				names = append(names, policy.Name)
			}
			assert.Contains(t, names, apiKeyAuthPolicyName, "the endpoint is not left open")
		})
	}
}

// Turning off the only variant without turning security off is contradictory and refused, rather than read as no
// security.
func TestPutEnv_APIKeyOffWithSecurityOn_400(t *testing.T) {
	enabled, disabled := true, false
	for name, security := range map[string]*models.MCPProxyEnvironmentSecurity{
		"security enabled": {Enabled: &enabled, APIKey: &models.APIKeySecurity{Enabled: &disabled}},
		"security omitted": {APIKey: &models.APIKeySecurity{Enabled: &disabled}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: security})
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Zero(t, f.endpointWrites, "nothing is stored")
			assert.Empty(t, f.deployed)
		})
	}
}

func TestPutEnv_NoSecurity_Accepted(t *testing.T) {
	disabled := false
	for name, security := range map[string]*models.MCPProxyEnvironmentSecurity{
		"enabled false":            {Enabled: &disabled},
		"enabled false, key given": {Enabled: &disabled, APIKey: &models.APIKeySecurity{Key: "X-Key"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			resp, created, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: security})
			require.NoError(t, err)
			assert.True(t, created)
			require.NotNil(t, resp.Security)
			require.NotNil(t, resp.Security.Enabled)
			assert.False(t, *resp.Security.Enabled)
			assert.Nil(t, resp.Security.APIKey)

			endpoint, ee := f.binding()
			require.NotNil(t, endpoint.Configuration.Security)
			assert.False(t, isBoolTrue(endpoint.Configuration.Security.Enabled), "stored as no security")
			assert.False(t, mcpProxyAPIKeySecurityEnabled(f.proxy, ee.EnvironmentUUID.String()),
				"agents binding it get only the URL, no API key")

			require.Len(t, f.deployed, 1)
			for _, policy := range deployedYAML(t, f.deployed[0]).Spec.Policies {
				assert.NotEqual(t, apiKeyAuthPolicyName, policy.Name, "a no-security binding renders no api-key policy")
			}

			// Identical again is a no-op.
			f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
			_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: security})
			require.NoError(t, err)
			assert.Len(t, f.deployed, 1)
		})
	}

	t.Run("the default renders the api-key policy", func(t *testing.T) {
		f := newBindingFixture(t)
		_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
		require.NoError(t, err)
		require.Len(t, f.deployed, 1)
		var names []string
		for _, policy := range deployedYAML(t, f.deployed[0]).Spec.Policies {
			names = append(names, policy.Name)
		}
		assert.Contains(t, names, apiKeyAuthPolicyName)
	})
}

func TestPutEnv_APIKeySecurityAndUpstreamAuthStored(t *testing.T) {
	enabled := true
	f := newBindingFixture(t)
	req := models.MCPProxyEnvironmentRequest{
		Security: &models.MCPProxyEnvironmentSecurity{
			Enabled: &enabled, APIKey: &models.APIKeySecurity{Enabled: &enabled, Key: "X-Weather-Key", In: "header"},
		},
		UpstreamAuth: &models.MCPProxyEnvironmentUpstreamAuth{Type: "api-key", Header: "Authorization", Value: "Bearer s3cret"},
		Policies:     []models.MCPPolicy{{Name: "basic-ratelimit", Version: "v1", Params: map[string]interface{}{"limit": float64(10)}}},
	}
	resp, _, err := f.put(t, req)
	require.NoError(t, err)
	require.NotNil(t, resp.UpstreamAuth)
	assert.Equal(t, "Authorization", resp.UpstreamAuth.Header)
	require.NotNil(t, resp.Security)
	require.NotNil(t, resp.Security.APIKey)
	assert.Equal(t, "X-Weather-Key", resp.Security.APIKey.Key)
	require.Len(t, resp.Policies, 1)

	endpoint, ee := f.binding()
	require.NotNil(t, endpoint.Configuration.Upstream.Auth)
	assert.Nil(t, endpoint.Configuration.Upstream.Auth.Value, "the credential is stored encrypted")
	require.NotNil(t, endpoint.Configuration.Upstream.Auth.SecretRef)

	// The same body again, credential included, is identical: no redeploy.
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	_, _, err = f.put(t, req)
	require.NoError(t, err)
	assert.Len(t, f.deployed, 1)

	// Omitting upstreamAuth resets it, which is a change.
	req.UpstreamAuth = nil
	_, _, err = f.put(t, req)
	require.NoError(t, err)
	assert.Len(t, f.deployed, 2)
	endpoint, _ = f.binding()
	assert.Nil(t, endpoint.Configuration.Upstream.Auth)
}

func TestPutEnv_ProxyFromOtherProject_404(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.svc.PutEnvironment(context.Background(), testOrg, "p2", "p2-uuid", bindingHandle, bindingEnv, models.MCPProxyEnvironmentRequest{})
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	_, err = f.svc.GetEnvironment(context.Background(), testOrg, "p2", "p2-uuid", bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	err = f.svc.DeleteEnvironment(context.Background(), testOrg, "p2", "p2-uuid", bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	assert.Zero(t, f.endpointWrites)
}

func TestPutEnv_UnknownEnvironment_404(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.svc.PutEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, "nope", models.MCPProxyEnvironmentRequest{})
	require.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	assert.Zero(t, f.endpointWrites)
}

func TestGetEnv_NotBound_404AndBoundReadsStatus(t *testing.T) {
	f := newBindingFixture(t)
	_, err := f.svc.GetEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyEnvironmentNotBound)

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed

	resp, err := f.svc.GetEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	assert.Equal(t, models.MCPDeploymentStatusDeployed, resp.DeploymentStatus)
	assert.Equal(t, "/weather", resp.Context)
	assert.Equal(t, "https://dev-gw.example.com/p1/weather/mcp", resp.PublicURL)

	proxy, err := f.svc.Get(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle)
	require.NoError(t, err)
	require.Len(t, proxy.Environments, 1, "the proxy GET lists its bindings")
	assert.Equal(t, *resp, proxy.Environments[0])
}

func TestPlacement_LateGatewayClash_MarksFailed(t *testing.T) {
	f := newBindingFixture(t)
	f.gateway = nil
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()

	// A gateway registers, already serving an artifact at an overlapping context, and the placement pass deploys
	// the binding there: the core refuses it.
	f.gateway = newGateway(t, models.GatewayRoleBoth, true)
	f.others[uuid.New()] = []byte("spec:\n  context: /p1\n")
	placement := models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: testOrg, ArtifactUUID: ee.ArtifactUUID, Kind: models.KindMCPMapping},
		GatewayUUID: f.gateway.UUID,
	}
	_, err = f.svc.base.core.Deploy(context.Background(), txSentinel, placement)
	require.ErrorIs(t, err, utils.ErrMCPContextInUse)
	assert.True(t, reconcile.IsPermanent(err), "a clash is given up at once, so the binding shows Failed")
	assert.Equal(t, "CONTEXT_IN_USE", f.svc.base.kind.DescribeRenderError(err))
	assert.Empty(t, f.deployed, "nothing is sent")

	// What a given-up PENDING row reads as.
	row := models.DeploymentStatusRecord{
		ArtifactUUID: ee.ArtifactUUID, GatewayUUID: f.gateway.UUID, Status: models.DeploymentStatusPending,
		StatusDesired: models.DeploymentStatusDeployed, RenderReason: f.svc.base.kind.DescribeRenderError(err),
	}
	assert.Equal(t, models.MCPDeploymentStatusFailed, models.MCPDeploymentStatusOf(models.UserStatusOf(row)))

	// An org proxy is checked the same way.
	f.proxy.ProjectUUID, f.proxy.ProjectName = nil, nil
	f.others[uuid.New()] = []byte("spec:\n  context: /weather/v2\n")
	_, err = f.svc.base.core.Deploy(context.Background(), txSentinel, placement)
	require.ErrorIs(t, err, utils.ErrMCPContextInUse)
}

// An agent mapped to the binding in its environment holds keys broadcast against the binding's artifact; deleting
// the binding would strand them, so the delete is refused as the proxy delete is. A mapping in another
// environment does not hold it, and the delete then locks the proxy row as PUT does.
func TestDeleteEnv_AgentMappedInEnvironment_409McpProxyInUse(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	require.NotNil(t, ee)

	var mappings []models.EnvAgentMCPMapping
	f.svc.base.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByMCPProxyFunc: func(_ context.Context, proxyUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			assert.Equal(t, f.proxy.UUID, proxyUUID)
			return mappings, nil
		},
	}
	repo := f.svc.base.repo.(*repomocks.MCPProxyRepositoryMock)
	locks := len(repo.GetByHandleForUpdateCalls())

	mappings = []models.EnvAgentMCPMapping{{ConfigUUID: uuid.New(), EnvironmentUUID: f.envUUID, MCPProxyUUID: f.proxy.UUID, ArtifactUUID: uuid.New()}}
	err = f.svc.DeleteEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyHasMappings)
	assert.Len(t, repo.GetByHandleForUpdateCalls(), locks, "a refused delete takes no lock and changes nothing")
	_, still := f.binding()
	assert.NotNil(t, still, "the binding is kept")

	// A mapping in another environment does not hold this binding; the delete goes on and locks the proxy row
	// (the fixture's lock reports the proxy gone, which ends the delete there).
	mappings = []models.EnvAgentMCPMapping{{ConfigUUID: uuid.New(), EnvironmentUUID: uuid.New(), MCPProxyUUID: f.proxy.UUID, ArtifactUUID: uuid.New()}}
	repo.GetByHandleForUpdateFunc = func(context.Context, *gorm.DB, string, string) (*models.MCPProxy, error) {
		return nil, gorm.ErrRecordNotFound
	}
	err = f.svc.DeleteEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	assert.NotErrorIs(t, err, utils.ErrMCPProxyHasMappings)
	assert.Len(t, repo.GetByHandleForUpdateCalls(), locks+1, "DELETE locks the proxy row like PUT")

	// A failing mapping lookup is a 500, not masked as in-use or not-found.
	f.svc.base.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByMCPProxyFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return nil, errors.New("db down")
		},
	}
	err = f.svc.DeleteEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrMCPProxyHasMappings)
	assert.NotErrorIs(t, err, utils.ErrMCPProxyNotFound)
}

// An environment deleted before endpoint rows were dropped with it left the binding's endpoint behind with no
// environment. Re-binding an environment of the same name reuses that row instead of colliding with its handle.
func TestPutEnv_ReusesOrphanedEndpointWithTheHandle(t *testing.T) {
	f := newBindingFixture(t)
	orphan := models.MCPProxyEndpoint{UUID: uuid.New(), MCPProxyUUID: f.proxy.UUID, Handle: bindingEnv, Name: bindingEnv, Status: models.StatusCreated}
	f.proxy.Endpoints = append(f.proxy.Endpoints, orphan)
	endpointRepo := f.svc.base.endpointRepo.(*repomocks.MCPProxyEndpointRepositoryMock)

	_, created, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Empty(t, endpointRepo.CreateEndpointCalls(), "the orphaned row is reused, not re-inserted")
	require.Len(t, f.proxy.Endpoints, 1)
	endpoint, ee := f.binding()
	require.NotNil(t, ee)
	assert.Equal(t, orphan.UUID, endpoint.UUID)
	require.NotNil(t, endpoint.Configuration.Upstream, "the reused row carries the new binding's config")
	assert.Equal(t, bindingServiceURL+"/mcp", endpoint.Configuration.Upstream.URL)
	require.Len(t, f.deployed, 1)
}

// Two egress gateways in the environment and no gatewayId is the contract's 400 AMBIGUOUS_GATEWAY, and stores
// nothing. Naming one of them resolves it.
func TestPutEnv_AmbiguousGateway_400AmbiguousGateway(t *testing.T) {
	f := newBindingFixture(t)
	second := newGateway(t, models.GatewayRoleBoth, true)
	gatewayRepo := f.svc.base.gatewayRepo.(*repomocks.GatewayRepositoryMock)
	gatewayRepo.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{f.gateway, second}, nil
	}

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.ErrorIs(t, err, utils.ErrMCPAmbiguousGateway)
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "still a 400 to any caller mapping invalid input")
	assert.Zero(t, f.endpointWrites, "nothing is stored")
	assert.Empty(t, f.deployed)

	id := f.gateway.UUID.String()
	_, created, err := f.put(t, models.MCPProxyEnvironmentRequest{GatewayID: &id})
	require.NoError(t, err)
	assert.True(t, created)
}

// A self-hosted gateway is customer-run and has no route to the in-cluster upstream: naming one is refused before
// anything is stored, and an environment whose only egress gateway is self-hosted stores the binding as no gateway.
func TestPutEnv_SelfHostedGateway(t *testing.T) {
	t.Run("named is a 400 and nothing is stored", func(t *testing.T) {
		f := newBindingFixture(t)
		f.gateway.Hosting = models.GatewayHostingSelfHosted
		id := f.gateway.UUID.String()
		_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{GatewayID: &id})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Contains(t, err.Error(), "managed gateways")
		assert.Zero(t, f.endpointWrites, "nothing is stored")
		assert.Empty(t, f.deployed)
	})
	t.Run("the only egress gateway is stored as no gateway", func(t *testing.T) {
		f := newBindingFixture(t)
		f.gateway.Hosting = models.GatewayHostingSelfHosted
		resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
		require.NoError(t, err)
		assert.Equal(t, "NO_GATEWAY", resp.DeploymentStatusReason)
		assert.Empty(t, f.deployed, "nothing is sent to the self-hosted gateway")
	})
}

func bindingTools(names ...string) *models.MCPProxyCapabilities {
	tools := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		tools = append(tools, map[string]interface{}{"name": name, "description": name + " tool"})
	}
	return &models.MCPProxyCapabilities{Tools: &tools}
}

func toolNamesOf(capabilities *models.MCPProxyCapabilities) []string {
	if capabilities == nil || capabilities.Tools == nil {
		return nil
	}
	names := []string{}
	for _, tool := range *capabilities.Tools {
		names = append(names, tool["name"].(string))
	}
	return names
}

// The capabilities a binding is given are stored on its endpoint, where every reader of an org proxy's tools
// already looks, and read back on the binding and on the proxy.
func TestPutEnv_CapabilitiesStoredAndReadBack(t *testing.T) {
	f := newBindingFixture(t)
	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: bindingTools("get_weather", "get_forecast")})
	require.NoError(t, err)
	assert.Equal(t, []string{"get_weather", "get_forecast"}, toolNamesOf(resp.Capabilities))

	endpoint, _ := f.binding()
	assert.Equal(t, []string{"get_weather", "get_forecast"}, toolNamesOf(endpoint.Configuration.Capabilities))

	got, err := f.svc.GetEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	assert.Equal(t, []string{"get_weather", "get_forecast"}, toolNamesOf(got.Capabilities))
}

func TestPutEnv_Capabilities_ChangeRedeploysIdenticalDoesNot(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: bindingTools("get_weather")})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	writes := f.endpointWrites

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: bindingTools("get_weather")})
	require.NoError(t, err)
	assert.Len(t, f.deployed, 1, "identical capabilities are a no-op")
	assert.Equal(t, writes, f.endpointWrites)

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: bindingTools("get_weather", "get_forecast")})
	require.NoError(t, err)
	assert.Len(t, f.deployed, 2, "a changed tool list is stored and redeployed")
	endpoint, _ := f.binding()
	assert.Equal(t, []string{"get_weather", "get_forecast"}, toolNamesOf(endpoint.Configuration.Capabilities))
}

// The body replaces the binding: capabilities left out, or given as empty lists, clear the stored ones.
func TestPutEnv_Capabilities_OmittedOrEmptyClears(t *testing.T) {
	empty := []map[string]interface{}{}
	for name, capabilities := range map[string]*models.MCPProxyCapabilities{
		"omitted":     nil,
		"empty lists": {Tools: &empty, Prompts: &empty},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: bindingTools("get_weather")})
			require.NoError(t, err)

			resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: capabilities})
			require.NoError(t, err)
			assert.Nil(t, resp.Capabilities)
			endpoint, _ := f.binding()
			assert.Nil(t, endpoint.Configuration.Capabilities)
		})
	}
}

func TestPutEnv_Capabilities_Invalid400NothingStored(t *testing.T) {
	tooMany := make([]string, maxBindingCapabilityEntries+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("tool_%d", i)
	}
	huge := []map[string]interface{}{{"name": "big", "description": strings.Repeat("x", maxBindingCapabilitiesBytes)}}
	noURI := []map[string]interface{}{{"name": "readme"}}
	for name, capabilities := range map[string]*models.MCPProxyCapabilities{
		"tool without a name":  {Tools: &[]map[string]interface{}{{"description": "nameless"}}},
		"tool with blank name": {Tools: &[]map[string]interface{}{{"name": "  "}}},
		"resource without uri": {Resources: &noURI},
		"too many tools":       bindingTools(tooMany...),
		"over the size limit":  {Tools: &huge},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBindingFixture(t)
			_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Capabilities: capabilities})
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Zero(t, f.endpointWrites, "nothing is stored")
			assert.Empty(t, f.deployed)
		})
	}
}

// An api-key upstream auth sent without its value keeps the stored one, so a binding read back (which never carries
// the value) can be sent again unchanged. With nothing stored to keep, the value is required.
func TestPutEnv_UpstreamAuthWithoutValueKeepsTheStoredOne(t *testing.T) {
	f := newBindingFixture(t)

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{
		UpstreamAuth: &models.MCPProxyEnvironmentUpstreamAuth{Type: "api-key", Header: "Authorization"},
	})
	require.ErrorIs(t, err, utils.ErrInvalidInput, "no stored value to keep")

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{
		UpstreamAuth: &models.MCPProxyEnvironmentUpstreamAuth{Type: "api-key", Header: "Authorization", Value: "Bearer s3cret"},
	})
	require.NoError(t, err)
	_, ee := f.binding()
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed
	deployed := len(f.deployed)

	// The same auth without its value is identical: no redeploy.
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{
		UpstreamAuth: &models.MCPProxyEnvironmentUpstreamAuth{Type: "api-key", Header: "Authorization"},
	})
	require.NoError(t, err)
	assert.Len(t, f.deployed, deployed, "unchanged")

	// A new header without a value keeps the value under the new header.
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{
		UpstreamAuth: &models.MCPProxyEnvironmentUpstreamAuth{Type: "api-key", Header: "X-Upstream-Key"},
	})
	require.NoError(t, err)
	endpoint, _ := f.binding()
	auth := endpoint.Configuration.Upstream.Auth
	require.NotNil(t, auth)
	assert.Equal(t, "X-Upstream-Key", valueOrEmpty(auth.Header))
	require.NotNil(t, auth.SecretRef)
	value, err := utils.DecryptSecretRef(*auth.SecretRef, f.svc.base.encryptionKey)
	require.NoError(t, err)
	assert.Equal(t, "Bearer s3cret", value)
}

// asURLProxy makes the fixture's proxy one registered by URL.
func (f *bindingFixture) asURLProxy(url string) {
	f.proxy.Configuration.Source = nil
	f.proxy.Configuration.Upstream = models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: url}}
}

// A binding of a proxy registered by URL calls that URL: no release binding is looked up.
func TestPutEnv_URLProxy_UsesItsURLWithoutAReleaseBinding(t *testing.T) {
	f := newBindingFixture(t)
	f.asURLProxy("https://8.8.8.8/mcp")
	f.serviceURLErr = errors.New("no release binding is looked up for a URL proxy")

	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	assert.Empty(t, f.releaseBindingNames, "no upstream lookup")
	endpoint, _ := f.binding()
	require.NotNil(t, endpoint.Configuration.Upstream)
	assert.Equal(t, "https://8.8.8.8/mcp", endpoint.Configuration.Upstream.URL)
	assert.NotEmpty(t, resp.PublicURL)

	name := "weather-dev"
	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{ReleaseBindingName: &name})
	require.ErrorIs(t, err, utils.ErrInvalidInput, "a release binding applies only to a sourced proxy")
}

// A URL proxy's upstream must be public: a managed gateway calls it as given.
func TestPutEnv_URLProxy_PrivateURL400NothingStored(t *testing.T) {
	f := newBindingFixture(t)
	f.asURLProxy("http://10.1.2.3:8080/mcp")

	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Empty(t, f.proxy.Endpoints, "nothing stored")
}

// Changing a URL proxy's URL points its bindings at the new one and redeploys them.
func TestUpsert_URLProxyURLChangeUpdatesItsBindings(t *testing.T) {
	f := newBindingFixture(t)
	f.asURLProxy("https://8.8.8.8/mcp")
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)

	ctxPath := "/weather"
	_, _, err = f.svc.Upsert(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle,
		models.ProjectMCPProxyRequest{
			Name: "Weather", Version: "v1", Context: ctxPath,
			Upstream: &models.ProjectMCPProxyUpstream{URL: "https://8.8.4.4/mcp"},
		})
	require.NoError(t, err)
	endpoint, _ := f.binding()
	assert.Equal(t, "https://8.8.4.4/mcp", endpoint.Configuration.Upstream.URL)
	assert.Positive(t, f.rerenders, "the bindings are redeployed")
}

// A binding created after the proxy was read, and before its URL change locked it, is redeployed with the new URL
// too: the redeploys come from the locked proxy's bindings, not the copy read first.
func TestUpsert_URLProxyURLChangeRedeploysABindingCreatedMeanwhile(t *testing.T) {
	f := newBindingFixture(t)
	f.asURLProxy("https://8.8.8.8/mcp")
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{})
	require.NoError(t, err)
	_, ee := f.binding()
	// The read the update starts from predates the binding.
	repo := f.svc.base.repo.(*repomocks.MCPProxyRepositoryMock)
	readFirst := repo.GetInProjectFunc
	repo.GetInProjectFunc = func(ctx context.Context, handle, org, project string) (*models.MCPProxy, error) {
		p, err := readFirst(ctx, handle, org, project)
		if p != nil {
			p.Endpoints = nil
		}
		return p, err
	}
	f.rerendered = nil

	_, _, err = f.svc.Upsert(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle,
		models.ProjectMCPProxyRequest{
			Name: "Weather", Version: "v1", Context: "/weather",
			Upstream: &models.ProjectMCPProxyUpstream{URL: "https://8.8.4.4/mcp"},
		})
	require.NoError(t, err)
	assert.Contains(t, f.rerendered, ee.ArtifactUUID, "the binding created meanwhile is redeployed")
}

func bindingTimeouts(timeout, idle string) *models.Resilience {
	return &models.Resilience{Timeout: &timeout, IdleTimeout: &idle}
}

// A binding's timeouts are stored on its endpoint, read back, and a change redeploys it while the same ones do not.
func TestPutEnv_ResilienceStoredReadBackAndRedeploysOnChange(t *testing.T) {
	f := newBindingFixture(t)
	resp, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Resilience: bindingTimeouts(" 15s ", "1m")})
	require.NoError(t, err)
	require.NotNil(t, resp.Resilience)
	assert.Equal(t, "15s", *resp.Resilience.Timeout)
	assert.Equal(t, "1m", *resp.Resilience.IdleTimeout)

	endpoint, ee := f.binding()
	require.NotNil(t, endpoint.Configuration.Resilience)
	assert.Equal(t, "15s", *endpoint.Configuration.Resilience.Timeout)
	f.status[ee.ArtifactUUID].Status = models.DeploymentStatusDeployed

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Resilience: bindingTimeouts("15s", "1m")})
	require.NoError(t, err)
	assert.Len(t, f.deployed, 1, "the same timeouts are a no-op")

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Resilience: bindingTimeouts("30s", "1m")})
	require.NoError(t, err)
	assert.Len(t, f.deployed, 2, "a changed timeout is stored and redeployed")

	resp, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Resilience: bindingTimeouts(" ", "")})
	require.NoError(t, err)
	assert.Nil(t, resp.Resilience, "blank timeouts clear them")
	endpoint, _ = f.binding()
	assert.Nil(t, endpoint.Configuration.Resilience)
}

func TestPutEnv_Resilience_Invalid400NothingStored(t *testing.T) {
	f := newBindingFixture(t)
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Resilience: bindingTimeouts("fifteen", "")})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Zero(t, f.endpointWrites, "nothing is stored")
	assert.Empty(t, f.deployed)
}

// The gateway artifact of an environment carries its endpoint's timeouts.
func TestBuildMCPProxyEnvArtifact_CarriesTheEndpointTimeouts(t *testing.T) {
	ctxPath, projectUUID, projectName := "/docs", "p-uuid", "p1"
	for name, proxy := range map[string]*models.MCPProxy{
		// Before, an org endpoint's stored timeouts were dropped from its artifact; they now reach the gateway.
		"org proxy": {UUID: uuid.New(), Handle: "docs", Configuration: models.MCPProxyConfig{Context: &ctxPath}},
		"project proxy": {
			UUID: uuid.New(), Handle: "docs", ProjectUUID: &projectUUID, ProjectName: &projectName,
			Configuration: models.MCPProxyConfig{Context: &ctxPath},
		},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := &models.MCPProxyEndpoint{
				Handle: "main",
				Configuration: models.MCPEndpointConfig{
					Upstream:   &models.UpstreamEndpoint{URL: "https://8.8.8.8/mcp"},
					Resilience: bindingTimeouts("15s", "1m"),
				},
			}
			ee := &models.MCPProxyEndpointEnvironment{ArtifactUUID: uuid.New(), EnvironmentUUID: uuid.New()}
			deployable := buildMCPProxyEnvArtifact(proxy, endpoint, ee)
			rendered, err := buildMCPProxyDeploymentYAML(deployable, "docs", nil)
			require.NoError(t, err)
			require.NotNil(t, rendered.Spec.Resilience)
			assert.Equal(t, "15s", *rendered.Spec.Resilience.Timeout)
			assert.Equal(t, "1m", *rendered.Spec.Resilience.IdleTimeout)
		})
	}
}

// The gateway reads a binding's API key from a header only, so a query location is refused rather than deployed.
func TestPutEnv_APIKeyInQuery_400NothingStored(t *testing.T) {
	f := newBindingFixture(t)
	enabled := true
	_, _, err := f.put(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{
		Enabled: &enabled, APIKey: &models.APIKeySecurity{Enabled: &enabled, Key: "api_key", In: "query"},
	}})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Zero(t, f.endpointWrites, "nothing is stored")
	assert.Empty(t, f.deployed)
}

// A binding saved while its proxy's URL changed takes the locked proxy's URL rather than writing the old one back; a
// move to another host is refused when the binding carries a credential given for the old host.
func TestFollowLockedProxyURL(t *testing.T) {
	apiKey, header, secret := "api-key", "X-Key", "s3cret"
	auth := &models.UpstreamAuth{Type: &apiKey, Header: &header, Value: &secret}
	config := func(withAuth bool) models.MCPEndpointConfig {
		upstream := &models.UpstreamEndpoint{URL: "https://8.8.8.8/mcp"}
		if withAuth {
			upstream.Auth = auth
		}
		return models.MCPEndpointConfig{Upstream: upstream}
	}
	proxyAt := func(url string) *models.MCPProxy {
		return &models.MCPProxy{Configuration: models.MCPProxyConfig{
			Upstream: models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: url}},
		}}
	}

	desired, _, err := followLockedProxyURL(proxyAt("https://8.8.8.8/mcp"), config(true), config(true))
	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/mcp", desired.Upstream.URL, "unchanged")

	desired, stored, err := followLockedProxyURL(proxyAt("https://8.8.8.8/v2/mcp"), config(true), config(true))
	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/v2/mcp", desired.Upstream.URL, "same host: the new URL is kept")
	assert.Equal(t, "https://8.8.8.8/v2/mcp", stored.Upstream.URL)
	assert.NotNil(t, stored.Upstream.Auth)

	_, _, err = followLockedProxyURL(proxyAt("https://1.1.1.1/mcp"), config(true), config(true))
	assert.ErrorIs(t, err, utils.ErrMCPProxyURLChanged, "another host with a credential")

	desired, _, err = followLockedProxyURL(proxyAt("https://1.1.1.1/mcp"), config(false), config(false))
	require.NoError(t, err)
	assert.Equal(t, "https://1.1.1.1/mcp", desired.Upstream.URL, "another host without a credential")

	sourced := proxyAt("https://1.1.1.1/mcp")
	sourced.Configuration.Source = &models.MCPProxySource{ComponentName: "c", EndpointName: "e"}
	desired, _, err = followLockedProxyURL(sourced, config(true), config(true))
	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/mcp", desired.Upstream.URL, "a sourced binding resolves its own upstream")
}
