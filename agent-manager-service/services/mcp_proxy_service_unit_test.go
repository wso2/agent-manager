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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// testMCPEnvUUID is a valid environment UUID used as an endpoint's target environment.
const testMCPEnvUUID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

// identityEnabledSecurity returns a SecurityConfig selecting the Agent Identity variant.
func identityEnabledSecurity() *models.SecurityConfig {
	return &models.SecurityConfig{
		Enabled:  boolPtr(true),
		Identity: &models.IdentitySecurity{Enabled: boolPtr(true)},
	}
}

// endpointWith builds a single-environment endpoint DTO targeting testMCPEnvUUID with the
// given upstream URL and security.
func endpointWith(url string, security *models.SecurityConfig) models.MCPProxyEndpointDTO {
	var upstream models.UpstreamConfig
	if url != "" {
		upstream.Main = &models.UpstreamEndpoint{URL: url}
	}
	return models.MCPProxyEndpointDTO{
		ID:           "primary",
		Upstream:     upstream,
		Security:     security,
		Environments: []models.MCPEndpointEnvironmentDTO{{EnvironmentUUID: testMCPEnvUUID}},
	}
}

// gatewayWithPolicyManifest builds a Gateway whose Manifest advertises the given
// name/version policy pairs, in the shape extractGatewayPolicyManifestItems walks.
func gatewayWithPolicyManifest(nameVersionPairs ...string) *models.Gateway {
	items := make([]interface{}, 0, len(nameVersionPairs)/2)
	for i := 0; i+1 < len(nameVersionPairs); i += 2 {
		items = append(items, map[string]interface{}{
			"name":    nameVersionPairs[i],
			"version": nameVersionPairs[i+1],
		})
	}
	return &models.Gateway{
		Manifest:                 map[string]interface{}{"policies": items},
		GatewayFunctionalityType: models.GatewayRoleBoth,
	}
}

func TestDefaultMCPProxySecurity_IdentityVariantSkipsAPIKeyDefaults(t *testing.T) {
	out := defaultMCPProxySecurity(&models.SecurityConfig{
		Enabled:  boolPtr(true),
		Identity: &models.IdentitySecurity{Enabled: boolPtr(true)},
	})
	assert.Nil(t, out.APIKey, "identity mode must not default an API key on")
	assert.NotNil(t, out.Identity)
	assert.True(t, isBoolTrue(out.Identity.Enabled))
}

func TestMCPProxyCreate_RejectsNonKebabHandle(t *testing.T) {
	svc := &MCPProxyService{}
	for _, id := range []string{"Bad_Handle", "UPPER", "has space", "trail-", "-lead", "a--b", strings.Repeat("a", 101)} {
		_, err := svc.Create(context.Background(), "org-uuid", "system",
			&models.MCPProxyDTO{ID: id, Name: "x", Version: "v1"})
		assert.ErrorIs(t, err, utils.ErrInvalidInput, "handle %q must be rejected", id)
		assert.Contains(t, err.Error(), "kebab-case", "handle %q must be rejected by the kebab check, not a later validation", id)
	}
}

func TestValidateMCPEndpoints_RejectsBothVariantsEnabled(t *testing.T) {
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", &models.SecurityConfig{
			Enabled:  boolPtr(true),
			APIKey:   &models.APIKeySecurity{Enabled: boolPtr(true)},
			Identity: &models.IdentitySecurity{Enabled: boolPtr(true)},
		}),
	}
	err := validateMCPEndpoints(endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpoints_RejectsDuplicateEnvironmentAcrossEndpoints(t *testing.T) {
	first := endpointWith("https://93.184.216.34", nil)
	first.ID = "primary"
	second := endpointWith("https://93.184.216.35", nil)
	second.ID = "secondary"
	err := validateMCPEndpoints([]models.MCPProxyEndpointDTO{first, second})
	assert.ErrorIs(t, err, utils.ErrMCPEnvAlreadyBound)
}

func TestValidateMCPEndpointSecurity_IdentityNeedsGatewayPolicies(t *testing.T) {
	// Gateway advertises mcp-auth but not mcp-authz.
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1")}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	err := svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpointSecurity_IdentityAcceptedWithGatewayPolicies(t *testing.T) {
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1", "mcp-authz", "v1")}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil))
}

// A candidate gateway that has not reported its manifest yet (e.g. a self-hosted gateway
// registered but never connected) is treated like no gateway: it doesn't block Agent
// Identity; its policies are checked once it reports.
func TestValidateMCPEndpointSecurity_IdentitySkipsGatewayThatHasNotReported(t *testing.T) {
	clearGatewayManifestCache(t)
	silent := newGateway(t, models.GatewayRoleEgress, true)
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1", "mcp-authz", "v1"), silent}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil))
}

// A gateway that has reported but lacks mcp-authz still refuses Agent Identity, even
// alongside one that has not reported.
func TestValidateMCPEndpointSecurity_IdentityRefusedByReportedGatewayDespiteSilentOne(t *testing.T) {
	clearGatewayManifestCache(t)
	silent := newGateway(t, models.GatewayRoleEgress, true)
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{silent, gatewayWithPolicyManifest("mcp-auth", "v1")}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	err := svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

// A gateway that has not reported its manifest must not empty the org's MCP policy list.
func TestListAvailableMCPPolicies_SkipsGatewayThatHasNotReported(t *testing.T) {
	clearGatewayManifestCache(t)
	reported := newGateway(t, models.GatewayRoleEgress, true)
	assert.NoError(t, gatewayManifestCache.Set(context.Background(), reported.UUID.String(),
		policyManifest("mcp-auth", "v1")))
	silent := newGateway(t, models.GatewayRoleEgress, true)
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{silent, reported}, nil
		},
	}

	resp, err := (&MCPProxyService{gatewayRepo: gwRepo}).ListAvailableMCPPolicies(context.Background(), "org1")

	assert.NoError(t, err)
	if assert.Len(t, resp.List, 1) {
		assert.Equal(t, "mcp-auth", resp.List[0].Name)
	}
}

func TestValidateMCPEndpointSecurity_IdentityAllowedWhenNoGatewayYet(t *testing.T) {
	// No active gateway for the environment yet: identity mode is allowed; policies
	// are re-checked once a gateway exists.
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil))
}

// newDeleteTestProxy builds a proxy with one identity-enabled endpoint bound to envUUID,
// ready for MCPProxyService.Delete's Thunder cleanup.
func newDeleteTestProxy(handle string, envUUID uuid.UUID) *models.MCPProxy {
	return &models.MCPProxy{
		UUID:      uuid.New(),
		Artifact:  &models.Artifact{Handle: handle},
		Endpoints: []models.MCPProxyEndpoint{identityEnabledEndpoint("primary", envUUID, uuid.Nil, true)},
	}
}

func TestMCPProxyDelete_CleansThunderResourceServers(t *testing.T) {
	envUUID := uuid.New()
	proxy := newDeleteTestProxy("gh-proxy", envUUID)

	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) { return proxy, nil },
		DeleteFunc:      func(_ context.Context, _, _ string) error { return nil },
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		ListEndpointEnvironmentsByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
			return nil, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}, {Action: "write"}}, nil
		},
	}
	infra := stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{Name: "env-a", UUID: envUUID.String()}}, nil
	}}

	var deletedHandle string
	type removedPermission struct {
		roleID string
		req    thundersvc.RolePermissionRequest
	}
	var removed []removedPermission
	envClient := &clientmocks.EnvIdentityClientMock{
		DeleteProxyResourceServerFunc: func(_ context.Context, proxyHandle string) error {
			deletedHandle = proxyHandle
			return nil
		},
		ListRolesFunc: func(_ context.Context, ouID string, offset, _ int) ([]thundersvc.ThunderRole, int, error) {
			assert.Equal(t, "", ouID, "role sweep must list every role in the env-Thunder, not filter by a platform OU")
			if offset > 0 {
				return nil, 1, nil
			}
			return []thundersvc.ThunderRole{
				{ID: "role-1", Permissions: []thundersvc.RolePermissionRequest{
					{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read", "gh-proxy:write"}},
				}},
			}, 1, nil
		},
		RemoveRolePermissionsFunc: func(_ context.Context, roleID string, req thundersvc.RolePermissionRequest) error {
			removed = append(removed, removedPermission{roleID: roleID, req: req})
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, ouID, _, envName string) (thundersvc.EnvIdentityClient, error) {
			assert.Equal(t, "org-uuid", ouID, "cleanup must resolve by ouID (orgUUID), not orgName")
			assert.Equal(t, "env-a", envName)
			return envClient, nil
		},
	}

	svc := &MCPProxyService{
		repo:              proxyRepo,
		endpointRepo:      endpointRepo,
		mcpProxyScopeRepo: scopeRepo,
		infraManager:      infra,
		resolver:          resolver,
		logger:            discardLogger(),
	}

	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy")

	assert.NoError(t, err)
	assert.Equal(t, "gh-proxy", deletedHandle)
	if assert.Len(t, removed, 2) {
		assert.Equal(t, thundersvc.RolePermissionRequest{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read"}}, removed[0].req)
		assert.Equal(t, thundersvc.RolePermissionRequest{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:write"}}, removed[1].req)
	}
}

func TestMCPProxyDelete_CleanupSurvivesResolverError(t *testing.T) {
	envUUID := uuid.New()
	proxy := newDeleteTestProxy("gh-proxy", envUUID)

	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, _, _ string) (*models.MCPProxy, error) { return proxy, nil },
		DeleteFunc:      func(_ context.Context, _, _ string) error { return nil },
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		ListEndpointEnvironmentsByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
			return nil, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}}, nil
		},
	}
	infra := stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{Name: "env-a", UUID: envUUID.String()}}, nil
	}}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, _, _, _ string) (thundersvc.EnvIdentityClient, error) {
			return nil, assert.AnError
		},
	}

	svc := &MCPProxyService{
		repo:              proxyRepo,
		endpointRepo:      endpointRepo,
		mcpProxyScopeRepo: scopeRepo,
		infraManager:      infra,
		resolver:          resolver,
		logger:            discardLogger(),
	}

	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy")

	assert.NoError(t, err, "Thunder cleanup is best-effort and must never fail the delete")
}

// gatewayWithPolicyManifestAndRole is gatewayWithPolicyManifest with a distinct UUID/Name
// and an explicit functionality role, so two-egress-gateway scenarios can tell the
// candidates apart (by UUID for anchoring, by Name for error messages).
func gatewayWithPolicyManifestAndRole(role string, nameVersionPairs ...string) *models.Gateway {
	gw := gatewayWithPolicyManifest(nameVersionPairs...)
	gw.UUID = uuid.New()
	gw.Name = "gateway-" + gw.UUID.String()
	gw.GatewayFunctionalityType = role
	return gw
}

func TestValidateMCPEndpointSecurity_TwoEgressGateways_AnchorsOnExistingDeployment(t *testing.T) {
	// The deployed gateway has full policy support; the other egress candidate lacks
	// mcp-authz. Environment-based selection requires every candidate to comply and
	// would therefore fail, so NoError here proves the probe anchored on the deployment.
	deployed := gatewayWithPolicyManifestAndRole(models.GatewayRoleBoth, "mcp-auth", "v1", "mcp-authz", "v1")
	other := gatewayWithPolicyManifestAndRole(models.GatewayRoleEgress, "mcp-auth", "v1")
	gwRepo := gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{deployed, other})

	artifactUUID := uuid.New()
	depRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, gotArtifactUUID uuid.UUID) ([]string, error) {
			assert.Equal(t, artifactUUID, gotArtifactUUID)
			return []string{deployed.UUID.String()}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo, deploymentRepo: depRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	existingArtifactByEnv := map[string]uuid.UUID{testMCPEnvUUID: artifactUUID}

	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, existingArtifactByEnv))
}

func TestValidateMCPEndpointSecurity_TwoEgressGateways_NoDeployment_AllCandidatesCompliant(t *testing.T) {
	// No existing deployment to anchor on (create, or a genuinely new binding): this is a
	// read-only policy probe, so two egress-capable candidates must not be ambiguous —
	// both support the required policies, so validation passes.
	gwA := gatewayWithPolicyManifestAndRole(models.GatewayRoleBoth, "mcp-auth", "v1", "mcp-authz", "v1")
	gwB := gatewayWithPolicyManifestAndRole(models.GatewayRoleEgress, "mcp-auth", "v1", "mcp-authz", "v1")
	gwRepo := gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{gwA, gwB})
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}

	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil))
}

func TestValidateMCPEndpointSecurity_TwoEgressGateways_NoDeployment_OneNonCompliantFails(t *testing.T) {
	// Same no-anchor situation, but one of the two candidates lacks mcp-authz. The probe
	// must fail (not silently pick the compliant one) and name the offending gateway.
	compliant := gatewayWithPolicyManifestAndRole(models.GatewayRoleBoth, "mcp-auth", "v1", "mcp-authz", "v1")
	nonCompliant := gatewayWithPolicyManifestAndRole(models.GatewayRoleEgress, "mcp-auth", "v1")
	gwRepo := gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{compliant, nonCompliant})
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}

	err := svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints, nil)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), nonCompliant.Name)
}

// endpointBoundToGateway builds a single-environment endpoint DTO targeting testMCPEnvUUID
// with the caller's gatewayId set (empty string leaves it unset).
func endpointBoundToGateway(gatewayID string) models.MCPProxyEndpointDTO {
	endpoint := endpointWith("https://93.184.216.34", nil)
	if gatewayID != "" {
		endpoint.Environments[0].GatewayID = &gatewayID
	}
	return endpoint
}

// placementService wires only what validateMCPEndpointPlacement reads.
func placementService(gwRepo repositories.GatewayRepository, depRepo repositories.DeploymentRepository) *MCPProxyService {
	return &MCPProxyService{gatewayRepo: gwRepo, deploymentRepo: depRepo, logger: discardLogger()}
}

// deployedTo stubs the artifact's deployed-gateway set.
func deployedTo(gatewayIDs ...string) *repomocks.DeploymentRepositoryMock {
	return &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return gatewayIDs, nil
		},
	}
}

// These cover the create/update pre-check that moves the four fatal placement errors ahead
// of the write. Before it, Create committed the proxy and then 400'd from the deploy step,
// so a client correcting its gatewayId and retrying the same POST hit 409 instead.
func TestValidateMCPEndpointPlacement(t *testing.T) {
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	ingress := newGateway(t, models.GatewayRoleIngress, true)

	t.Run("naming an ingress-only gateway fails before any write", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{ingress, egress}), nil)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(ingress.UUID.String())}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorIs(t, err, errInvalidEgressGateway)
	})

	t.Run("two egress candidates and no gatewayId is ambiguous", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorIs(t, err, errAmbiguousEgressGateway)
	})

	t.Run("naming a valid candidate passes", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(egress.UUID.String())}, nil)
		assert.NoError(t, err)
	})

	t.Run("moving an already-deployed binding is placement-fixed", func(t *testing.T) {
		// The update path's anchor: the environment's artifact is deployed to `both`, and the
		// caller names `egress`. Surfacing this pre-write is what the anchor argument buys.
		artifactUUID := uuid.New()
		svc := placementService(
			gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}),
			deployedTo(both.UUID.String()),
		)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(egress.UUID.String())},
			map[string]uuid.UUID{testMCPEnvUUID: artifactUUID})
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorIs(t, err, errPlacementFixed)
	})

	t.Run("redeploying to the environment's current gateway passes", func(t *testing.T) {
		artifactUUID := uuid.New()
		svc := placementService(
			gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}),
			deployedTo(both.UUID.String()),
		)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(both.UUID.String())},
			map[string]uuid.UUID{testMCPEnvUUID: artifactUUID})
		assert.NoError(t, err)
	})

	t.Run("no gateway mapped to the environment is tolerated", func(t *testing.T) {
		// Timing condition, not misconfiguration: the deploy step skips and retries later.
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, nil), nil)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")}, nil)
		assert.NoError(t, err)
	})

	t.Run("an unresolvable anchor skips the pre-check rather than guessing", func(t *testing.T) {
		// The anchor lookup fails, so the requested gateway cannot be judged against it.
		// Resolving without the anchor would report ambiguity for a binding the anchor may
		// well have resolved cleanly, so this must defer to the deploy step, not 400.
		depRepo := &repomocks.DeploymentRepositoryMock{
			PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
				return nil, errors.New("connection refused")
			},
		}
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), depRepo)
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")},
			map[string]uuid.UUID{testMCPEnvUUID: uuid.New()})
		assert.NoError(t, err)
	})

	t.Run("a malformed environment UUID is left to endpoint validation", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		endpoint := endpointBoundToGateway("")
		endpoint.Environments[0].EnvironmentUUID = "not-a-uuid"
		_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil, []models.MCPProxyEndpointDTO{endpoint}, nil)
		assert.NoError(t, err)
	})
}

func TestMCPDeployErrorIsFatal_NilError(t *testing.T) {
	assert.False(t, mcpDeployErrorIsFatal(nil))
}

func TestMCPDeployErrorIsFatal_ToleratedError(t *testing.T) {
	// Only errNoGatewayForEnvironment is tolerated.
	assert.False(t, mcpDeployErrorIsFatal(errNoGatewayForEnvironment))
}

func TestMCPDeployErrorIsFatal_FatalErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"errNoEgressGatewayForEnvironment", errNoEgressGatewayForEnvironment},
		{"errAmbiguousEgressGateway", errAmbiguousEgressGateway},
		{"errInvalidEgressGateway", errInvalidEgressGateway},
		{"errPlacementFixed", errPlacementFixed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, mcpDeployErrorIsFatal(tt.err), "error %q must be fatal", tt.name)
		})
	}
}

func TestMCPDeployErrorIsFatal_JoinedErrorsMixed(t *testing.T) {
	// errors.Join of tolerated + fatal: the fatal takes precedence, returns true.
	joinedMixedErrors := errors.Join(errNoGatewayForEnvironment, errInvalidEgressGateway)
	assert.True(t, mcpDeployErrorIsFatal(joinedMixedErrors),
		"joined error with both tolerated and fatal must return true (fatal wins)")
}

func TestMCPDeployErrorIsFatal_JoinedErrorsToleratedOnly(t *testing.T) {
	// errors.Join of only tolerated errors: returns false.
	joinedToleratedOnly := errors.Join(errNoGatewayForEnvironment, errNoGatewayForEnvironment)
	assert.False(t, mcpDeployErrorIsFatal(joinedToleratedOnly),
		"joined error with only tolerated errors must return false")
}

func TestMCPDeployErrorIsFatal_UnrelatedError(t *testing.T) {
	// An error unrelated to any sentinel: returns false (not a fatal placement error).
	unrelatedErr := errors.New("some other error")
	assert.False(t, mcpDeployErrorIsFatal(unrelatedErr),
		"unrelated error must not be fatal")
}

// -----------------------------------------------------------------------------
// mapMCPProxyWriteError — maps Postgres unique-violation constraints from the
// proxy write path to friendly sentinels.
// -----------------------------------------------------------------------------

func TestMapMCPProxyWriteError_ProxyEnvSingleViolation(t *testing.T) {
	svc := &MCPProxyService{}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_proxy_env_single"}

	mapped := svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid")

	assert.ErrorIs(t, mapped, utils.ErrMCPEnvAlreadyBound)
}

func TestMapMCPProxyWriteError_EndpointEnvViolation(t *testing.T) {
	svc := &MCPProxyService{}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_endpoint_env"}

	mapped := svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid")

	assert.ErrorIs(t, mapped, utils.ErrMCPEnvAlreadyBound)
}

func TestMapMCPProxyWriteError_HandleConflictWithMCPProxy(t *testing.T) {
	svc := &MCPProxyService{
		artifactRepo: &repomocks.ArtifactRepositoryMock{
			GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
				return &models.Artifact{Kind: models.KindMCPProxy}, nil
			},
		},
	}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_artifact_handle_ou_id"}

	mapped := svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid")

	assert.ErrorIs(t, mapped, utils.ErrMCPProxyExists)
}

func TestMapMCPProxyWriteError_HandleConflictWithNonMCPArtifact(t *testing.T) {
	svc := &MCPProxyService{
		artifactRepo: &repomocks.ArtifactRepositoryMock{
			GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
				return &models.Artifact{Kind: models.KindLLMProvider}, nil
			},
		},
	}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_artifact_handle_ou_id"}

	mapped := svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid")

	// The conflicting artifact is an LLM provider, not an MCP proxy: must not
	// falsely report "MCP proxy already exists".
	assert.ErrorIs(t, mapped, utils.ErrArtifactExists)
	assert.NotErrorIs(t, mapped, utils.ErrMCPProxyExists)
}

func TestMapMCPProxyWriteError_HandleConflictLookupFailureFallsBackToMCPProxyExists(t *testing.T) {
	svc := &MCPProxyService{
		artifactRepo: &repomocks.ArtifactRepositoryMock{
			GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
				return nil, gorm.ErrRecordNotFound
			},
		},
	}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_artifact_handle_ou_id"}

	mapped := svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid")

	assert.ErrorIs(t, mapped, utils.ErrMCPProxyExists)
}

func TestMapMCPProxyWriteError_NameVersionConflict(t *testing.T) {
	svc := &MCPProxyService{}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "uq_artifact_name_version_ou_id"}

	mapped := svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid")

	assert.ErrorIs(t, mapped, utils.ErrInvalidInput)
}

func TestMapMCPProxyWriteError_UnrecognizedConstraintReturnsNil(t *testing.T) {
	svc := &MCPProxyService{}
	err := &pgconn.PgError{Code: "23505", ConstraintName: "some_other_constraint"}

	assert.Nil(t, svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid"))
}

func TestMapMCPProxyWriteError_NonUniqueViolationReturnsNil(t *testing.T) {
	svc := &MCPProxyService{}
	err := &pgconn.PgError{Code: "23503", ConstraintName: "uq_proxy_env_single"}

	assert.Nil(t, svc.mapMCPProxyWriteError(err, "some-handle", "org-uuid"))
}

func TestMapMCPProxyWriteError_NonPgErrorReturnsNil(t *testing.T) {
	svc := &MCPProxyService{}
	assert.Nil(t, svc.mapMCPProxyWriteError(errors.New("plain error"), "some-handle", "org-uuid"))
}

// TestSanitizeMCPUpstreamAuthForResponse_StripsSecretRef pins the sanitizer to the
// contract its caller documents. SecretRef holds the AES-256-GCM ciphertext of the
// upstream credential and is not declared on the spec's UpstreamAuth, so leaving it
// populated shipped that ciphertext to every caller with MCP proxy read permission.
func TestSanitizeMCPUpstreamAuthForResponse_StripsSecretRef(t *testing.T) {
	sanitized := sanitizeMCPUpstreamAuthForResponse(&models.UpstreamAuth{
		Type:      strPtr("api-key"),
		Header:    strPtr("Authorization"),
		Value:     strPtr("plaintext-upstream-key"),
		SecretRef: strPtr("BASE64-AES-GCM-CIPHERTEXT"),
	})

	assert.Nil(t, sanitized.Value)
	assert.Nil(t, sanitized.SecretRef)
	assert.Equal(t, "api-key", *sanitized.Type)
	assert.Equal(t, "Authorization", *sanitized.Header)
}

// TestSanitizeMCPUpstreamAuthForResponse_LeavesCallerIntact guards the copy: the
// sanitizer runs on the persisted configuration, which the write path still needs.
func TestSanitizeMCPUpstreamAuthForResponse_LeavesCallerIntact(t *testing.T) {
	auth := &models.UpstreamAuth{
		Type:      strPtr("api-key"),
		SecretRef: strPtr("BASE64-AES-GCM-CIPHERTEXT"),
	}

	sanitizeMCPUpstreamAuthForResponse(auth)

	assert.NotNil(t, auth.SecretRef)
	assert.Equal(t, "BASE64-AES-GCM-CIPHERTEXT", *auth.SecretRef)
}

// TestBuildMCPEndpointDTOsForResponse_StripsUpstreamCredentials exercises the path
// the proxy handlers actually serialize, not just the sanitizer in isolation.
func TestBuildMCPEndpointDTOsForResponse_StripsUpstreamCredentials(t *testing.T) {
	dtos := buildMCPEndpointDTOsForResponse([]models.MCPProxyEndpoint{
		{
			Handle: "primary",
			Name:   "Primary",
			Configuration: models.MCPEndpointConfig{
				Upstream: &models.UpstreamEndpoint{
					URL: "https://upstream.example",
					Auth: &models.UpstreamAuth{
						Type:      strPtr("api-key"),
						SecretRef: strPtr("BASE64-AES-GCM-CIPHERTEXT"),
					},
				},
			},
		},
	})

	assert.Len(t, dtos, 1)
	assert.Nil(t, dtos[0].Upstream.Main.Auth.Value)
	assert.Nil(t, dtos[0].Upstream.Main.Auth.SecretRef)
}

const privateMCPUpstream = "http://10.0.0.5/mcp"

func selfHostedGateway(t *testing.T) *models.Gateway {
	t.Helper()
	gw := newGateway(t, models.GatewayRoleBoth, true)
	gw.Hosting = models.GatewayHostingSelfHosted
	return gw
}

func privateUpstreamEndpoint(gatewayID string) models.MCPProxyEndpointDTO {
	endpoint := endpointBoundToGateway(gatewayID)
	endpoint.Upstream.Main.URL = privateMCPUpstream
	return endpoint
}

func TestMCPPrivateUpstream_SelfHostedOnlyAccepted(t *testing.T) {
	selfHosted := selfHostedGateway(t)
	endpoints := []models.MCPProxyEndpointDTO{privateUpstreamEndpoint(selfHosted.UUID.String())}
	svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{selfHosted}), nil)

	assert.NoError(t, validateMCPEndpoints(endpoints))
	_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil, endpoints, nil)
	assert.NoError(t, err)
}

func TestMCPUpstream_ShapeCheckedOnSelfHosted(t *testing.T) {
	for _, upstream := range []string{"ftp://10.0.0.5/mcp", "http://user:pw@10.0.0.5/mcp", "http:///mcp", "not a url", "http://10.0.0.5:0/mcp", "http://10.0.0.5:70000/mcp"} {
		endpoint := privateUpstreamEndpoint("")
		endpoint.Upstream.Main.URL = upstream
		err := validateMCPEndpoints([]models.MCPProxyEndpointDTO{endpoint})
		assert.ErrorIs(t, err, utils.ErrInvalidURL, "upstream %q", upstream)
	}
}

func TestMCPPrivateUpstream_ManagedTargetRefused(t *testing.T) {
	selfHosted := selfHostedGateway(t)
	managed := newGateway(t, models.GatewayRoleBoth, true)
	svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{selfHosted, managed}), nil)

	_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
		[]models.MCPProxyEndpointDTO{privateUpstreamEndpoint("")}, nil)
	assert.ErrorIs(t, err, utils.ErrInvalidURL)
}

func TestMCPPrivateUpstream_UpdateAddingManagedRefused(t *testing.T) {
	selfHosted := selfHostedGateway(t)
	managed := newGateway(t, models.GatewayRoleBoth, true)
	otherEnv := "7b1f2f0e-0d0c-4e9a-9a53-6a0c5f1d2b11"
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(f repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			if *f.EnvironmentID == testMCPEnvUUID {
				return []*models.Gateway{selfHosted}, nil
			}
			return []*models.Gateway{managed}, nil
		},
	}
	selfHostedID := selfHosted.UUID.String()
	endpoint := privateUpstreamEndpoint(selfHostedID)
	endpoint.Environments = append(endpoint.Environments, models.MCPEndpointEnvironmentDTO{EnvironmentUUID: otherEnv})
	svc := placementService(repo, nil)

	_, err := svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil, []models.MCPProxyEndpointDTO{endpoint},
		map[string]uuid.UUID{})
	assert.ErrorIs(t, err, utils.ErrInvalidURL)
}

func TestMCPPrivateUpstream_UnresolvedTargetsFailClosed(t *testing.T) {
	selfHosted := selfHostedGateway(t)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	both := newGateway(t, models.GatewayRoleBoth, true)
	ctx := context.Background()

	t.Run("no gateway mapped yet", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, nil), nil)
		_, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil, []models.MCPProxyEndpointDTO{privateUpstreamEndpoint("")}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidURL)
	})

	t.Run("anchor lookup fails", func(t *testing.T) {
		depRepo := &repomocks.DeploymentRepositoryMock{
			PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
				return nil, errors.New("connection refused")
			},
		}
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{selfHosted}), depRepo)
		_, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil, []models.MCPProxyEndpointDTO{privateUpstreamEndpoint(selfHosted.UUID.String())},
			map[string]uuid.UUID{testMCPEnvUUID: uuid.New()})
		assert.ErrorIs(t, err, utils.ErrInvalidURL)
	})

	t.Run("malformed environment id", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{selfHosted}), nil)
		endpoint := privateUpstreamEndpoint("")
		endpoint.Environments[0].EnvironmentUUID = "not-a-uuid"
		_, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil, []models.MCPProxyEndpointDTO{endpoint}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidURL)
	})

	t.Run("none named and inference is ambiguous", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		_, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil, []models.MCPProxyEndpointDTO{privateUpstreamEndpoint("")}, nil)
		assert.Error(t, err)
	})

	t.Run("gateway listing fails", func(t *testing.T) {
		repo := &repomocks.GatewayRepositoryMock{
			ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
				return nil, errors.New("db down")
			},
		}
		svc := placementService(repo, nil)
		_, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil, []models.MCPProxyEndpointDTO{privateUpstreamEndpoint(selfHosted.UUID.String())}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidURL)
	})
}

func TestMCPUpstreamNeedsSSRFGuard(t *testing.T) {
	selfHosted := &models.Gateway{Hosting: models.GatewayHostingSelfHosted}
	managed := &models.Gateway{}
	assert.False(t, mcpUpstreamNeedsSSRFGuard([]*models.Gateway{selfHosted, selfHosted}, false))
	assert.True(t, mcpUpstreamNeedsSSRFGuard([]*models.Gateway{selfHosted, managed}, false))
	assert.True(t, mcpUpstreamNeedsSSRFGuard([]*models.Gateway{selfHosted}, true))
	assert.True(t, mcpUpstreamNeedsSSRFGuard(nil, true))
	assert.True(t, mcpUpstreamNeedsSSRFGuard(nil, false), "no targets must not skip the guard")
}

func TestValidateMCPEndpointPlacement_ReturnsExemptGateways(t *testing.T) {
	selfHosted := selfHostedGateway(t)
	managed := newGateway(t, models.GatewayRoleBoth, true)
	ctx := context.Background()

	t.Run("an endpoint only on a self-hosted gateway is exempt there", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{selfHosted}), nil)
		exempt, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{privateUpstreamEndpoint(selfHosted.UUID.String())}, nil)
		assert.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{testMCPEnvUUID: selfHosted.UUID}, exempt)
	})

	t.Run("an endpoint on a managed gateway is not exempt", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{managed}), nil)
		exempt, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(managed.UUID.String())}, nil)
		assert.NoError(t, err)
		assert.Empty(t, exempt)

		_, err = svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{privateUpstreamEndpoint(managed.UUID.String())}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidURL)
	})

	t.Run("an endpoint with an unresolved environment is not exempt", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, nil), nil)
		exempt, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")}, nil)
		assert.NoError(t, err)
		assert.Empty(t, exempt)

		_, err = svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{privateUpstreamEndpoint("")}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidURL)
	})
}

func TestPinExemptGateways_FillsOnlyBlankGatewayIDs(t *testing.T) {
	const (
		blankEnv    = "11111111-1111-4111-8111-111111111111"
		explicitEnv = "22222222-2222-4222-8222-222222222222"
		otherEnv    = "33333333-3333-4333-8333-333333333333"
	)
	selfHosted := uuid.New()
	explicit := uuid.New().String()
	whitespace := "  "
	endpoints := []models.MCPProxyEndpointDTO{{
		ID: "primary",
		Environments: []models.MCPEndpointEnvironmentDTO{
			{EnvironmentUUID: blankEnv, GatewayID: &whitespace},
			{EnvironmentUUID: explicitEnv, GatewayID: &explicit},
			{EnvironmentUUID: otherEnv},
		},
	}}

	pinned := pinExemptGateways(endpoints, map[string]uuid.UUID{blankEnv: selfHosted, explicitEnv: selfHosted})

	envs := pinned[0].Environments
	assert.Equal(t, selfHosted.String(), *envs[0].GatewayID, "a blank gatewayId is pinned")
	assert.Equal(t, explicit, *envs[1].GatewayID, "an explicit gatewayId is kept")
	assert.Nil(t, envs[2].GatewayID, "a non-exempt environment is untouched")

	original := endpoints[0].Environments
	assert.Equal(t, "  ", *original[0].GatewayID, "the input is not mutated")
	assert.Same(t, &whitespace, original[0].GatewayID)
	assert.Nil(t, original[2].GatewayID)
}

// uuid.Parse accepts other spellings of an environment UUID, and the request body is not normalised before validation.
func TestValidateMCPEndpointPlacement_ExemptGatewaysWhateverTheEnvironmentUUIDSpelling(t *testing.T) {
	selfHosted := selfHostedGateway(t)
	managed := newGateway(t, models.GatewayRoleBoth, true)
	ctx := context.Background()
	upperCaseEnv := func(gatewayID string) models.MCPProxyEndpointDTO {
		endpoint := privateUpstreamEndpoint(gatewayID)
		endpoint.Environments[0].EnvironmentUUID = strings.ToUpper(testMCPEnvUUID)
		return endpoint
	}

	t.Run("a named self-hosted gateway is exempt under the canonical key", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{selfHosted}), nil)
		exempt, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{upperCaseEnv(selfHosted.UUID.String())}, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{testMCPEnvUUID: selfHosted.UUID}, exempt)
	})

	t.Run("an environment anchored on a self-hosted gateway is exempt and pinned", func(t *testing.T) {
		svc := placementService(
			gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{managed, selfHosted}),
			deployedTo(selfHosted.UUID.String()),
		)
		endpoints := []models.MCPProxyEndpointDTO{upperCaseEnv("")}

		exempt, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil, endpoints,
			map[string]uuid.UUID{testMCPEnvUUID: uuid.New()})
		require.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{testMCPEnvUUID: selfHosted.UUID}, exempt)

		pinned := pinExemptGateways(endpoints, exempt)
		require.NotNil(t, pinned[0].Environments[0].GatewayID)
		assert.Equal(t, selfHosted.UUID.String(), *pinned[0].Environments[0].GatewayID)
	})

	t.Run("the environment's deployed artifact still anchors it", func(t *testing.T) {
		svc := placementService(
			gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{managed, selfHosted}),
			deployedTo(managed.UUID.String()),
		)
		_, err := svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{upperCaseEnv(selfHosted.UUID.String())},
			map[string]uuid.UUID{testMCPEnvUUID: uuid.New()})
		assert.ErrorIs(t, err, errPlacementFixed)
	})
}

// Validation refuses before the transaction opens, so the service needs no database.
func TestMCPCreate_PrivateUpstreamOnManagedGatewayStillRejected(t *testing.T) {
	managed := newGateway(t, models.GatewayRoleBoth, true)
	svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{managed}), nil)

	_, err := svc.Create(context.Background(), "org1", "user", &models.MCPProxyDTO{
		ID: "weather", Name: "Weather", Version: "v1.0",
		Endpoints: []models.MCPProxyEndpointDTO{privateUpstreamEndpoint("")},
	})

	assert.ErrorIs(t, err, utils.ErrInvalidURL)
}
