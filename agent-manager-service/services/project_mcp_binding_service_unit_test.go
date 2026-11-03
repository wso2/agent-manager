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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestMCPUpstreamURL(t *testing.T) {
	tests := map[string]string{
		"http://svc:8080":  "http://svc:8080/mcp",
		"http://svc:8080/": "http://svc:8080/mcp",
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

	// status rows by artifact UUID; StartDeploy writes a DEPLOYING one.
	status map[uuid.UUID]*models.DeploymentStatusRecord
	// deployed holds every deployment StartDeploy was asked to send, in order.
	deployed []*models.Deployment
	// others are artifacts already deployed on the gateway: deployment id -> content.
	others map[uuid.UUID][]byte

	endpointWrites int
	hub            *stubEventHub
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
					f.proxy.Endpoints[i].Configuration = e.Configuration
					return nil
				}
			}
			return gorm.ErrRecordNotFound
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
		LockStatusRowsFunc:         func(context.Context, *gorm.DB, string, []uuid.UUID) error { return nil },
		LockEnvironmentMappingFunc: mappedEverywhere,
		StartDeployFunc: func(ctx context.Context, tx *gorm.DB, p models.Placement, d *models.Deployment) error {
			if err := startDeployStub(ctx, tx, p, d); err != nil {
				return err
			}
			id := uuid.New()
			d.DeploymentID = id
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
						out = append(out, models.GatewayDeploymentContent{DeploymentID: id, Content: d.Content})
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
	base.core = reconcile.NewCore(nil, deployments, f.hub, reconcile.Kinds{base.kind}, reconcile.Config{}, discardLogger())

	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			if name != bindingEnv {
				return nil, utils.ErrNotFound
			}
			return &models.EnvironmentResponse{UUID: f.envUUID.String(), Name: name}, nil
		},
		GetReleaseBindingEndpointServiceURLFunc: func(_ context.Context, _, projectName, component, endpoint, env string) (string, error) {
			assert.Equal(t, bindingProject, projectName, "the release binding lookup is always project-scoped")
			assert.Equal(t, "weather", component)
			assert.Equal(t, "mcp", endpoint)
			assert.Equal(t, bindingEnv, env)
			return f.serviceURL, f.serviceURLErr
		},
	}
	f.svc = NewProjectMCPProxyService(base, oc)
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
	assert.Equal(t, bindingServiceURL+"/mcp", endpoint.Configuration.Upstream.URL, "the stored upstream is the resolved URL plus /mcp")
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

	_, _, err = f.put(t, models.MCPProxyEnvironmentRequest{Security: &models.MCPProxyEnvironmentSecurity{
		Enabled: &enabled, Identity: &models.IdentitySecurity{Enabled: &enabled},
	}})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Zero(t, f.endpointWrites)
}

func TestPutEnv_NoSecurity_Accepted(t *testing.T) {
	enabled, disabled := true, false
	for name, security := range map[string]*models.MCPProxyEnvironmentSecurity{
		"enabled false": {Enabled: &disabled},
		"no variant":    {Enabled: &enabled},
		"apiKey off":    {Enabled: &enabled, APIKey: &models.APIKeySecurity{Enabled: &disabled}},
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

	proxy, err := f.svc.Get(context.Background(), testOrg, bindingProjectUUID, bindingHandle)
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

	// A gateway registers, already serving an artifact at an overlapping context, and the placement pass puts the
	// binding's PENDING row on it.
	f.gateway = newGateway(t, models.GatewayRoleBoth, true)
	f.others[uuid.New()] = []byte("spec:\n  context: /p1\n")
	f.status[ee.ArtifactUUID] = &models.DeploymentStatusRecord{
		ArtifactUUID: ee.ArtifactUUID, GatewayUUID: f.gateway.UUID,
		Status: models.DeploymentStatusPending, StatusDesired: models.DeploymentStatusDeployed,
	}

	ref := models.ArtifactRef{OUID: testOrg, ArtifactUUID: ee.ArtifactUUID, Kind: models.KindMCPMapping}
	_, err = f.svc.base.kind.Render(context.Background(), ref)
	require.ErrorIs(t, err, utils.ErrMCPContextInUse)
	assert.True(t, reconcile.IsPermanent(err), "a clash is given up at once, so the binding shows Failed")
	assert.Equal(t, "CONTEXT_IN_USE", f.svc.base.kind.DescribeRenderError(err))

	// What a given-up PENDING row reads as.
	row := *f.status[ee.ArtifactUUID]
	row.RenderReason = f.svc.base.kind.DescribeRenderError(err)
	assert.Equal(t, models.MCPDeploymentStatusFailed, models.MCPDeploymentStatusOf(models.UserStatusOf(row)))

	resp, err := f.svc.GetEnvironment(context.Background(), testOrg, bindingProject, bindingProjectUUID, bindingHandle, bindingEnv)
	require.NoError(t, err)
	assert.Equal(t, models.MCPDeploymentStatusDeploying, resp.DeploymentStatus, "before the outcome is recorded")

	// An org proxy's render never runs the check.
	f.proxy.ProjectUUID, f.proxy.ProjectName = nil, nil
	_, err = f.svc.base.kind.Render(context.Background(), ref)
	require.NoError(t, err)
}
