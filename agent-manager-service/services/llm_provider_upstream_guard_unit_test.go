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

// UNIT tests that an LLM provider's upstream cannot point a managed gateway at an internal address.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const guardOUID = "ou-upstream-guard"

// internalUpstreams are literal addresses, so the real check needs no DNS.
var internalUpstreams = map[string]string{
	"cloud metadata": "http://169.254.169.254/latest/meta-data/",
	"loopback":       "http://127.0.0.1:9090/v1",
	"localhost":      "http://localhost:8080",
	"private range":  "http://10.0.12.5/v1",
	"IPv6 loopback":  "http://[::1]:8080",
}

// allowAnyProviderUpstream stubs the public-host check for a test whose upstream is a real hostname, so it does no
// DNS lookup.
func allowAnyProviderUpstream(t *testing.T) {
	t.Helper()
	orig := validatePublicUpstreamURL
	validatePublicUpstreamURL = func(context.Context, string) error { return nil }
	t.Cleanup(func() { validatePublicUpstreamURL = orig })
}

func guardGateway(hosting string) *models.Gateway {
	return &models.Gateway{
		UUID: uuid.New(), OUID: guardOUID, Name: "gw-" + hosting, Hosting: hosting,
		GatewayFunctionalityType: models.GatewayRoleEgress,
	}
}

func guardProvider(upstreamURL string) *models.LLMProvider {
	return &models.LLMProvider{
		UUID:           uuid.New(),
		TemplateHandle: "openai",
		Configuration: models.LLMProviderConfig{
			Name: "p", Handle: "p", Version: "v1.0", Template: "openai",
			Upstream: &models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: upstreamURL}},
		},
	}
}

// renderableProviderAt is the golden provider, complete enough to render, with its upstream set to upstreamURL.
func renderableProviderAt(upstreamURL string) *models.LLMProvider {
	provider := goldenProvider()
	provider.Configuration.Upstream.Main.URL = upstreamURL
	return provider
}

func requireRefusedUpstream(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, utils.ErrInvalidURL)
	require.ErrorIs(t, err, utils.ErrInvalidInput, "existing handlers map invalid input to 400")
}

func TestRequirePublicUpstreamOn(t *testing.T) {
	for name, upstream := range internalUpstreams {
		t.Run(name+" is refused on a managed gateway", func(t *testing.T) {
			requireRefusedUpstream(t, requirePublicUpstreamOn(context.Background(), guardGateway(models.GatewayHostingManaged), upstream))
		})
		t.Run(name+" is refused on a gateway with no hosting recorded", func(t *testing.T) {
			requireRefusedUpstream(t, requirePublicUpstreamOn(context.Background(), guardGateway(""), upstream))
		})
		t.Run(name+" is allowed on a self-hosted gateway", func(t *testing.T) {
			require.NoError(t, requirePublicUpstreamOn(context.Background(), guardGateway(models.GatewayHostingSelfHosted), upstream))
		})
	}

	t.Run("a public address is allowed on a managed gateway", func(t *testing.T) {
		require.NoError(t, requirePublicUpstreamOn(context.Background(), guardGateway(models.GatewayHostingManaged), "https://8.8.8.8/v1"))
	})
	t.Run("a ref-only upstream has nothing to check", func(t *testing.T) {
		require.NoError(t, requirePublicUpstreamOn(context.Background(), guardGateway(models.GatewayHostingManaged), ""))
	})
	t.Run("the refusal does not echo the resolver's detail", func(t *testing.T) {
		err := requirePublicUpstreamOn(context.Background(), guardGateway(models.GatewayHostingManaged), "http://10.0.12.5/v1")
		assert.NotContains(t, err.Error(), "10.0.12.5")
		assert.NotContains(t, err.Error(), "non-public")
	})
}

func TestValidateProviderUpstreamShape(t *testing.T) {
	for _, bad := range []string{"ftp://example.com", "file:///etc/passwd", "https://user:pw@example.com", "not a url"} {
		requireRefusedUpstream(t, validateProviderUpstreamShape(guardProvider(bad)))
	}
	// Shape only: a private address is valid until the provider is deployed somewhere managed.
	require.NoError(t, validateProviderUpstreamShape(guardProvider("http://10.0.12.5/v1")))
	require.NoError(t, validateProviderUpstreamShape(guardProvider("")))
}

func TestRenderedUpstreamURLReadsTheRenderedProvider(t *testing.T) {
	content, err := renderLLMProvider(renderableProviderAt("http://169.254.169.254/latest"))
	require.NoError(t, err)

	got, err := renderedUpstreamURL([]byte(content))

	require.NoError(t, err)
	assert.Equal(t, "http://169.254.169.254/latest", got)
}

// deployGuardService is an LLMProviderDeploymentService whose gateway and provider lookups return the given ones;
// the deployment repo fails the test unless the test stubs what it may reach.
func deployGuardService(gateway *models.Gateway, provider *models.LLMProvider) (*LLMProviderDeploymentService, *repomocks.DeploymentRepositoryMock) {
	deployments := &repomocks.DeploymentRepositoryMock{}
	return &LLMProviderDeploymentService{
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			GetByUUIDFunc: func(string) (*models.Gateway, error) { return gateway, nil },
		},
		providerRepo: &repomocks.LLMProviderRepositoryMock{
			GetByUUIDFunc: func(string, string) (*models.LLMProvider, error) { return provider, nil },
		},
		deploymentRepo: deployments,
	}, deployments
}

var errReachedDeploy = errors.New("reached the deploy step")

func TestDeployLLMProvider_InternalUpstreamOnManagedGatewayIsRefused(t *testing.T) {
	gateway := guardGateway(models.GatewayHostingManaged)
	provider := guardProvider(internalUpstreams["cloud metadata"])
	svc, deployments := deployGuardService(gateway, provider)

	_, err := svc.DeployLLMProvider(provider.UUID.String(), &models.DeployAPIRequest{GatewayID: gateway.UUID.String()}, guardOUID)

	requireRefusedUpstream(t, err)
	assert.Empty(t, deployments.PlacedGatewaysCalls(), "a refused deploy must stop before placement")
}

func TestDeployLLMProvider_InternalUpstreamOnSelfHostedGatewayProceeds(t *testing.T) {
	gateway := guardGateway(models.GatewayHostingSelfHosted)
	provider := guardProvider(internalUpstreams["private range"])
	svc, deployments := deployGuardService(gateway, provider)
	deployments.PlacedGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) { return nil, errReachedDeploy }

	_, err := svc.DeployLLMProvider(provider.UUID.String(), &models.DeployAPIRequest{GatewayID: gateway.UUID.String()}, guardOUID)

	require.ErrorIs(t, err, errReachedDeploy)
}

func TestRestoreLLMProviderDeployment_InternalSnapshotOnManagedGatewayIsRefused(t *testing.T) {
	gateway := guardGateway(models.GatewayHostingManaged)
	current := guardProvider("https://api.openai.com/v1")
	snapshot, err := renderLLMProvider(renderableProviderAt(internalUpstreams["loopback"]))
	require.NoError(t, err)
	svc, deployments := deployGuardService(gateway, current)
	deployments.GetWithContentFunc = func(string, string, string) (*models.Deployment, error) {
		return &models.Deployment{DeploymentID: uuid.New(), GatewayUUID: gateway.UUID, Content: []byte(snapshot)}, nil
	}

	_, err = svc.RestoreLLMProviderDeployment(current.UUID.String(), uuid.NewString(), gateway.UUID.String(), guardOUID)

	requireRefusedUpstream(t, err)
	assert.Empty(t, deployments.GetStatusRecordCalls(), "a refused restore must stop before touching the deployment")
}

// updateGuardService is an LLMProviderService whose stored provider is existing, placed on gateway. Its repo
// update returns errReachedDeploy, so a test can tell the update got past the upstream check.
func updateGuardService(existing *models.LLMProvider, gateway *models.Gateway) (*LLMProviderService, *repomocks.LLMProviderRepositoryMock, *repomocks.DeploymentRepositoryMock) {
	providers := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(string, string) (*models.LLMProvider, error) { return existing, nil },
		UpdateFunc: func(context.Context, *models.LLMProvider, string, string) error {
			return errReachedDeploy
		},
	}
	deployments := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{gateway.UUID.String()}, nil
		},
	}
	return &LLMProviderService{
		providerRepo:   providers,
		deploymentRepo: deployments,
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			GetByUUIDFunc: func(string) (*models.Gateway, error) { return gateway, nil },
		},
	}, providers, deployments
}

func TestLLMProviderUpdate_InternalUpstreamOnPlacedManagedGatewayIsRefused(t *testing.T) {
	existing := guardProvider("https://8.8.8.8/v1")
	svc, providers, _ := updateGuardService(existing, guardGateway(models.GatewayHostingManaged))
	updates := guardProvider(internalUpstreams["cloud metadata"])
	updates.Configuration.Template = ""

	_, err := svc.Update(context.Background(), existing.UUID.String(), guardOUID, updates)

	requireRefusedUpstream(t, err)
	assert.Empty(t, providers.UpdateCalls(), "a refused upstream must not be stored")
}

func TestLLMProviderUpdate_InternalUpstreamOnSelfHostedGatewayIsStored(t *testing.T) {
	existing := guardProvider("http://10.0.0.1/v1")
	svc, _, _ := updateGuardService(existing, guardGateway(models.GatewayHostingSelfHosted))
	updates := guardProvider(internalUpstreams["private range"])
	updates.Configuration.Template = ""

	_, err := svc.Update(context.Background(), existing.UUID.String(), guardOUID, updates)

	require.ErrorIs(t, err, errReachedDeploy)
}

func TestLLMProviderUpdate_UnchangedUpstreamIsNotRechecked(t *testing.T) {
	// A row stored before the rule stays editable.
	existing := guardProvider(internalUpstreams["loopback"])
	svc, _, deployments := updateGuardService(existing, guardGateway(models.GatewayHostingManaged))
	updates := guardProvider(internalUpstreams["loopback"])
	updates.Configuration.Template = ""

	_, err := svc.Update(context.Background(), existing.UUID.String(), guardOUID, updates)

	require.ErrorIs(t, err, errReachedDeploy)
	assert.Empty(t, deployments.PlacedGatewaysCalls())
}

func TestLLMProviderCreate_MalformedUpstreamIsRefused(t *testing.T) {
	templates := NewLLMTemplateStore()
	templates.Load([]*models.LLMProviderTemplate{{Handle: "openai"}})
	svc := &LLMProviderService{
		providerRepo: &repomocks.LLMProviderRepositoryMock{
			GetByHandleFunc: func(string, string) (*models.LLMProvider, error) { return nil, gorm.ErrRecordNotFound },
		},
		templateStore: templates,
	}

	err := svc.prepareProvider(guardOUID, "me", guardProvider("file:///etc/passwd"))

	requireRefusedUpstream(t, err)
}

func TestLLMProviderCreateAndDeploy_InternalUpstreamOnManagedGatewayIsRefused(t *testing.T) {
	gateway := guardGateway(models.GatewayHostingManaged)
	templates := NewLLMTemplateStore()
	templates.Load([]*models.LLMProviderTemplate{{Handle: "openai"}})
	svc := &LLMProviderService{
		providerRepo: &repomocks.LLMProviderRepositoryMock{
			GetByHandleFunc: func(string, string) (*models.LLMProvider, error) { return nil, gorm.ErrRecordNotFound },
		},
		templateStore: templates,
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			GetByUUIDFunc: func(string) (*models.Gateway, error) { return gateway, nil },
		},
	}

	_, err := svc.CreateAndDeploy(context.Background(), guardOUID, "me", guardProvider(internalUpstreams["cloud metadata"]),
		[]string{gateway.UUID.String()}, nil)

	requireRefusedUpstream(t, err)
}
