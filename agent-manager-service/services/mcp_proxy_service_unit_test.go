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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
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
	err := validateMCPEndpoints(context.Background(), endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpoints_RejectsDuplicateEnvironmentAcrossEndpoints(t *testing.T) {
	first := endpointWith("https://93.184.216.34", nil)
	first.ID = "primary"
	second := endpointWith("https://93.184.216.35", nil)
	second.ID = "secondary"
	err := validateMCPEndpoints(context.Background(), []models.MCPProxyEndpointDTO{first, second})
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
		GetDeployedGatewaysByProviderFunc: func(gotArtifactUUID uuid.UUID, _ string) ([]string, error) {
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
		GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
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
		err := svc.validateMCPEndpointPlacement("org1",
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(ingress.UUID.String())}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorIs(t, err, errInvalidEgressGateway)
	})

	t.Run("two egress candidates and no gatewayId is ambiguous", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		err := svc.validateMCPEndpointPlacement("org1",
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")}, nil)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorIs(t, err, errAmbiguousEgressGateway)
	})

	t.Run("naming a valid candidate passes", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		err := svc.validateMCPEndpointPlacement("org1",
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
		err := svc.validateMCPEndpointPlacement("org1",
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
		err := svc.validateMCPEndpointPlacement("org1",
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(both.UUID.String())},
			map[string]uuid.UUID{testMCPEnvUUID: artifactUUID})
		assert.NoError(t, err)
	})

	t.Run("no gateway mapped to the environment is tolerated", func(t *testing.T) {
		// Timing condition, not misconfiguration: the deploy step skips and retries later.
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, nil), nil)
		err := svc.validateMCPEndpointPlacement("org1",
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")}, nil)
		assert.NoError(t, err)
	})

	t.Run("an unresolvable anchor skips the pre-check rather than guessing", func(t *testing.T) {
		// The anchor lookup fails, so the requested gateway cannot be judged against it.
		// Resolving without the anchor would report ambiguity for a binding the anchor may
		// well have resolved cleanly, so this must defer to the deploy step, not 400.
		depRepo := &repomocks.DeploymentRepositoryMock{
			GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
				return nil, errors.New("connection refused")
			},
		}
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), depRepo)
		err := svc.validateMCPEndpointPlacement("org1",
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway("")},
			map[string]uuid.UUID{testMCPEnvUUID: uuid.New()})
		assert.NoError(t, err)
	})

	t.Run("a malformed environment UUID is left to endpoint validation", func(t *testing.T) {
		svc := placementService(gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{both, egress}), nil)
		endpoint := endpointBoundToGateway("")
		endpoint.Environments[0].EnvironmentUUID = "not-a-uuid"
		err := svc.validateMCPEndpointPlacement("org1", []models.MCPProxyEndpointDTO{endpoint}, nil)
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

// oldMCPServerForTest is a fake old MCP server (2025-xx). It refuses server/discover the way the
// Go SDK v1.5 does, and answers initialize and tools/list. It records what it received.
type oldMCPServerForTest struct {
	// refuseDiscoverInJSONRPC makes it refuse server/discover with HTTP 200 and a JSON-RPC
	// "Method not found" error, the other way an old server can refuse.
	refuseDiscoverInJSONRPC bool
	// discoverStatus and discoverBody, when set, make it answer server/discover with that HTTP
	// status and body instead.
	discoverStatus int
	discoverBody   string

	methods        []string // the JSON-RPC method of each request, in order
	discoverHeader http.Header
	discoverMeta   map[string]interface{}
}

func (f *oldMCPServerForTest) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params struct {
			Meta map[string]interface{} `json:"_meta"`
		} `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.methods = append(f.methods, req.Method)

	switch req.Method {
	case "server/discover":
		f.discoverHeader = r.Header.Clone()
		f.discoverMeta = req.Params.Meta
		if f.discoverStatus != 0 {
			w.WriteHeader(f.discoverStatus)
			_, _ = w.Write([]byte(f.discoverBody))
			return
		}
		if f.refuseDiscoverInJSONRPC {
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32601,"message":"Method not found"}}`, req.ID)
			return
		}
		http.Error(w, "Bad Request: Unsupported protocol version", http.StatusBadRequest)
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "session-1")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"old-server"}}}`, req.ID)
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[{"name":"greet_old"}]}}`, req.ID)
	default:
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{}}`, req.ID)
	}
}

// Agent Manager asks server/discover first, in the MCP 2026-07-28 format. A server before
// 2026-07-28 refuses it with an HTTP error, and the fetch carries on with initialize.
func TestFetchMCPServerInfo_AsksServerDiscoverFirst(t *testing.T) {
	fake := &oldMCPServerForTest{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	// The real client refuses localhost (SSRF guard), so use the test server's own client.
	svc := &MCPProxyService{client: srv.Client(), logger: discardLogger()}

	resp, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

	// The old server refused server/discover, but the fetch still works the old way.
	assert.NoError(t, err)
	if assert.NotNil(t, resp.Tools) {
		assert.Equal(t, "greet_old", (*resp.Tools)[0]["name"])
	}

	// server/discover is sent first, then the old flow.
	wantOrder := []string{
		"server/discover",
		"initialize", "notifications/initialized",
		"tools/list", "prompts/list", "resources/list",
	}
	assert.Equal(t, wantOrder, fake.methods)

	// server/discover uses the new format: headers and params._meta.
	assert.Equal(t, "2026-07-28", fake.discoverHeader.Get("MCP-Protocol-Version"))
	assert.Equal(t, "server/discover", fake.discoverHeader.Get("Mcp-Method"))
	assert.Equal(t, "2026-07-28", fake.discoverMeta["io.modelcontextprotocol/protocolVersion"])
	assert.NotNil(t, fake.discoverMeta["io.modelcontextprotocol/clientInfo"])
	assert.NotNil(t, fake.discoverMeta["io.modelcontextprotocol/clientCapabilities"])
}

// A server before MCP 2026-07-28 can also refuse server/discover with HTTP 200 and a JSON-RPC
// error. That still counts as a refusal, so Agent Manager connects with initialize.
func TestFetchMCPServerInfo_OldServerRefusesDiscoverInJSONRPC(t *testing.T) {
	fake := &oldMCPServerForTest{refuseDiscoverInJSONRPC: true}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	svc := &MCPProxyService{client: srv.Client(), logger: discardLogger()}

	resp, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

	assert.NoError(t, err)
	if assert.NotNil(t, resp.Tools) {
		assert.Equal(t, "greet_old", (*resp.Tools)[0]["name"])
	}
	assert.Equal(t, []string{"server/discover", "initialize"}, fake.methods[:2])
}

// newMCPServerForTest is a fake new MCP server (2026-07-28 only). It answers server/discover,
// refuses initialize (new servers don't have it), and is strict: it refuses any request that
// lacks the new format (params._meta and the MCP-Protocol-Version and Mcp-Method headers).
// It records what it received.
type newMCPServerForTest struct {
	methods    []string // the JSON-RPC method of each request, in order
	sessionIDs []string // the Mcp-Session-Id header of each request, in order
}

func (f *newMCPServerForTest) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params struct {
			Meta map[string]interface{} `json:"_meta"`
		} `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.methods = append(f.methods, req.Method)
	f.sessionIDs = append(f.sessionIDs, r.Header.Get("Mcp-Session-Id"))

	hasNewFormat := r.Header.Get("MCP-Protocol-Version") == "2026-07-28" &&
		r.Header.Get("Mcp-Method") == req.Method &&
		req.Params.Meta["io.modelcontextprotocol/protocolVersion"] == "2026-07-28"
	if !hasNewFormat {
		http.Error(w, "Bad Request: missing MCP 2026-07-28 headers or _meta", http.StatusBadRequest)
		return
	}

	switch req.Method {
	case "server/discover":
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"supportedVersions":["2026-07-28"],"capabilities":{"tools":{}},`+
			`"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"new-server","version":"1.0.0"}}}}`, req.ID)
	case "tools/list":
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"resultType":"complete","tools":[{"name":"greet_new"}]}}`, req.ID)
	case "prompts/list", "resources/list":
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"resultType":"complete"}}`, req.ID)
	default:
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32601,"message":"Method not found"}}`, req.ID)
	}
}

// A server that answers server/discover speaks MCP 2026-07-28. Agent Manager never sends it
// initialize, and lists its tools, prompts and resources in the 2026-07-28 format: params._meta
// and the MCP-Protocol-Version and Mcp-Method headers on every request, and no session.
func TestFetchMCPServerInfo_NewServerListsToolsTheNewWay(t *testing.T) {
	fake := &newMCPServerForTest{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	svc := &MCPProxyService{client: srv.Client(), logger: discardLogger()}

	resp, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

	assert.NoError(t, err)
	if assert.NotNil(t, resp.Tools) {
		assert.Equal(t, "greet_new", (*resp.Tools)[0]["name"])
	}
	// The server's name comes from the server/discover answer.
	if assert.NotNil(t, resp.ServerInfo) {
		assert.Equal(t, "new-server", (*resp.ServerInfo)["name"])
	}
	// No initialize, and no notifications/initialized: straight from discover to the lists.
	assert.Equal(t, []string{"server/discover", "tools/list", "prompts/list", "resources/list"}, fake.methods)
	// No session on any request.
	assert.Equal(t, []string{"", "", "", ""}, fake.sessionIDs)
}

// MCP versions are dates, so the check compares dates. An empty version counts as old.
func TestMCPVersionNeedsInitialize(t *testing.T) {
	assert.True(t, mcpVersionNeedsInitialize("2025-06-18"))
	assert.True(t, mcpVersionNeedsInitialize("2025-11-25"))
	assert.False(t, mcpVersionNeedsInitialize("2026-07-28"))
	assert.False(t, mcpVersionNeedsInitialize("2027-01-01"))
	assert.True(t, mcpVersionNeedsInitialize(""))
}

// When server/discover fails, Agent Manager tries initialize only if the server refused the
// method. Credential, rate-limit and server problems are reported instead: initialize would
// fail the same way and hide the real error.
func TestFetchMCPServerInfo_WhenToFallBackToInitialize(t *testing.T) {
	jsonRPCError := `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`
	tests := []struct {
		name           string
		discoverStatus int
		discoverBody   string
		wantFallBack   bool
		wantErrorIs    error // checked only when not falling back; nil means any error
	}{
		{name: "404 refuses the method", discoverStatus: 404, discoverBody: "not found", wantFallBack: true},
		{name: "405 refuses the method", discoverStatus: 405, discoverBody: "method not allowed", wantFallBack: true},
		{name: "500 with a JSON-RPC error refuses the method", discoverStatus: 500, discoverBody: jsonRPCError, wantFallBack: true},
		{name: "400 with an old server's own JSON-RPC error refuses the method", discoverStatus: 400, discoverBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"Bad Request: Server not initialized"}}`, wantFallBack: true},
		{name: "400 unsupported protocol version (-32022) is reported", discoverStatus: 400, discoverBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32022,"message":"Unsupported protocol version","data":{"supported":["2027-03-01"],"requested":"2026-07-28"}}}`},
		{name: "400 header mismatch (-32020) is reported", discoverStatus: 400, discoverBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32020,"message":"Header mismatch"}}`},
		{name: "401 is reported", discoverStatus: 401, discoverBody: "unauthorized", wantErrorIs: utils.ErrMCPServerUnauthorized},
		{name: "403 is reported", discoverStatus: 403, discoverBody: "forbidden"},
		{name: "429 is reported", discoverStatus: 429, discoverBody: "slow down"},
		{name: "500 without JSON-RPC is reported", discoverStatus: 500, discoverBody: "internal error"},
		{name: "200 that is not JSON-RPC is reported", discoverStatus: 200, discoverBody: "<html>sign in</html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &oldMCPServerForTest{discoverStatus: tt.discoverStatus, discoverBody: tt.discoverBody}
			srv := httptest.NewServer(fake)
			defer srv.Close()
			svc := &MCPProxyService{client: srv.Client(), logger: discardLogger()}

			resp, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

			if tt.wantFallBack {
				assert.NoError(t, err)
				assert.Equal(t, []string{"server/discover", "initialize"}, fake.methods[:2])
				if assert.NotNil(t, resp.Tools) {
					assert.Equal(t, "greet_old", (*resp.Tools)[0]["name"])
				}
				return
			}
			assert.Error(t, err)
			if tt.wantErrorIs != nil {
				assert.ErrorIs(t, err, tt.wantErrorIs)
			}
			assert.Equal(t, []string{"server/discover"}, fake.methods, "initialize must not be sent")
		})
	}
}

// A server that can't be reached is reported as unreachable, without trying initialize.
func TestFetchMCPServerInfo_UnreachableServerIsReported(t *testing.T) {
	srv := httptest.NewServer(&oldMCPServerForTest{})
	client := srv.Client()
	srv.Close() // nothing listens on the URL any more

	svc := &MCPProxyService{client: client, logger: discardLogger()}
	_, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

	assert.ErrorIs(t, err, utils.ErrURLUnreachable)
}

// The status error keeps the text it had before it became a type.
func TestMCPHTTPStatusError_KeepsOldText(t *testing.T) {
	err := &mcpHTTPStatusError{StatusCode: 403, Body: "forbidden"}
	assert.Equal(t, "request failed with status 403: forbidden", err.Error())
}

func TestChooseMCPVersion(t *testing.T) {
	tests := []struct {
		name           string
		serverVersions []string
		want           string
	}{
		{name: "highest shared version", serverVersions: []string{"2025-11-25", "2026-07-28"}, want: "2026-07-28"},
		{name: "only versions before 2026-07-28", serverVersions: []string{"2025-06-18", "2025-11-25"}, want: "2025-11-25"},
		{name: "versions Agent Manager doesn't support are skipped", serverVersions: []string{"2027-03-01", "2026-07-28"}, want: "2026-07-28"},
		{name: "empty list: none", serverVersions: []string{}, want: ""},
		{name: "no version in common: none", serverVersions: []string{"2027-03-01"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chooseMCPVersion(tt.serverVersions))
		})
	}
}

// A server can answer server/discover yet give no 2026-07-28 or later version Agent Manager
// supports. Like the Go MCP SDK's client, Agent Manager then connects with initialize.
func TestFetchMCPServerInfo_DiscoverAnsweredButInitializeStillNeeded(t *testing.T) {
	tests := []struct {
		name              string
		supportedVersions string
	}{
		{name: "only versions before 2026-07-28", supportedVersions: `["2025-11-25","2025-06-18"]`},
		{name: "empty list", supportedVersions: `[]`},
		{name: "no version in common", supportedVersions: `["2027-03-01"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &oldMCPServerForTest{
				discoverStatus: 200,
				discoverBody:   `{"jsonrpc":"2.0","id":1,"result":{"supportedVersions":` + tt.supportedVersions + `}}`,
			}
			srv := httptest.NewServer(fake)
			defer srv.Close()
			svc := &MCPProxyService{client: srv.Client(), logger: discardLogger()}

			resp, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

			assert.NoError(t, err)
			if assert.NotNil(t, resp.Tools) {
				assert.Equal(t, "greet_old", (*resp.Tools)[0]["name"])
			}
			assert.Equal(t, []string{"server/discover", "initialize"}, fake.methods[:2])
		})
	}
}

// The fetch response reports which MCP versions the server supports, so the proxy can be
// configured with them.
func TestFetchMCPServerInfo_ReportsSupportedVersions(t *testing.T) {
	tests := []struct {
		name   string
		server http.Handler
		want   *[]string
	}{
		{
			name:   "server before 2026-07-28: the version initialize agreed on",
			server: &oldMCPServerForTest{},
			want:   &[]string{"2025-06-18"},
		},
		{
			name:   "server on 2026-07-28: the versions server/discover reported",
			server: &newMCPServerForTest{},
			want:   &[]string{"2026-07-28"},
		},
		{
			name: "server/discover answered with only earlier versions: that list, not just initialize's one",
			server: &oldMCPServerForTest{
				discoverStatus: 200,
				discoverBody:   `{"jsonrpc":"2.0","id":1,"result":{"supportedVersions":["2025-11-25","2025-06-18"]}}`,
			},
			want: &[]string{"2025-11-25", "2025-06-18"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.server)
			defer srv.Close()
			svc := &MCPProxyService{client: srv.Client(), logger: discardLogger()}

			resp, err := svc.fetchMCPServerInfo(context.Background(), srv.URL, "", "")

			assert.NoError(t, err)
			assert.Equal(t, tt.want, resp.SupportedVersions)
		})
	}
}

// Only values shaped like an MCP version (a date) are reported; none left means no field.
func TestObtainValidMCPVersionsFromServer(t *testing.T) {
	assert.Equal(t, &[]string{"2026-07-28", "2025-06-18"},
		obtainValidMCPVersionsFromServer([]string{"2026-07-28", "latest", "", "2025-06-18", "2026-7-28"}))
	assert.Nil(t, obtainValidMCPVersionsFromServer([]string{"latest"}))
	assert.Nil(t, obtainValidMCPVersionsFromServer(nil))
}

func TestValidateMCPSpecVersionsInRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     models.MCPProxyDTO
		wantErr bool
	}{
		{name: "neither set", req: models.MCPProxyDTO{}},
		{name: "only the deprecated single version", req: models.MCPProxyDTO{McpSpecVersion: strPtr("2025-06-18")}},
		{name: "a list of versions", req: models.MCPProxyDTO{McpSpecVersions: &[]string{"2026-07-28", "2025-11-25"}}},
		{name: "both set", req: models.MCPProxyDTO{McpSpecVersion: strPtr("2025-06-18"), McpSpecVersions: &[]string{"2026-07-28"}}, wantErr: true},
		{name: "empty list", req: models.MCPProxyDTO{McpSpecVersions: &[]string{}}, wantErr: true},
		{name: "a value that isn't a date", req: models.MCPProxyDTO{McpSpecVersions: &[]string{"2026-07-28", "latest"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMCPSpecVersionsInRequest(tt.req.McpSpecVersion, tt.req.McpSpecVersions)
			if tt.wantErr {
				assert.ErrorIs(t, err, utils.ErrInvalidInput)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// A proxy saved before mcpSpecVersions existed has only the single version; it reads back as a
// one-item list.
func TestObtainStoredMCPSpecVersions(t *testing.T) {
	assert.Equal(t, &[]string{"2026-07-28", "2025-11-25"},
		obtainStoredMCPSpecVersions(models.MCPProxyConfig{SpecVersions: []string{"2026-07-28", "2025-11-25"}}))
	assert.Equal(t, &[]string{"2025-06-18"},
		obtainStoredMCPSpecVersions(models.MCPProxyConfig{SpecVersion: "2025-06-18"}))
	assert.Nil(t, obtainStoredMCPSpecVersions(models.MCPProxyConfig{}))
}

// The proxy and list-item responses both return the versions as mcpSpecVersions only; the
// deprecated mcpSpecVersion is not returned, even for a proxy saved before the list existed.
func TestConvertModelMCPProxy_ReturnsMCPSpecVersions(t *testing.T) {
	for name, config := range map[string]models.MCPProxyConfig{
		"saved with a list":             {SpecVersions: []string{"2026-07-28"}},
		"saved before the list existed": {SpecVersion: "2026-07-28"},
	} {
		t.Run(name, func(t *testing.T) {
			proxy := &models.MCPProxy{Configuration: config}

			assert.Equal(t, &[]string{"2026-07-28"}, convertModelMCPProxyToSpec(proxy).McpSpecVersions)
			assert.Nil(t, convertModelMCPProxyToSpec(proxy).McpSpecVersion)
			assert.Equal(t, &[]string{"2026-07-28"}, convertModelMCPProxyToListItem(proxy).McpSpecVersions)
			assert.Nil(t, convertModelMCPProxyToListItem(proxy).McpSpecVersion)
		})
	}
}

// The console edits a proxy by sending back what GET returned, with its changes. That must pass
// the version check, including for a proxy saved before the list existed.
func TestConvertModelMCPProxy_ResponseSentBackPassesVersionCheck(t *testing.T) {
	got := convertModelMCPProxyToSpec(&models.MCPProxy{Configuration: models.MCPProxyConfig{SpecVersion: "2025-06-18"}})

	assert.NoError(t, validateMCPSpecVersionsInRequest(got.McpSpecVersion, got.McpSpecVersions))
}

// Only the list is saved: a request using the deprecated single mcpSpecVersion is saved as a
// one-item list.
func TestObtainMCPSpecVersionsFromRequest(t *testing.T) {
	assert.Equal(t, []string{"2026-07-28", "2025-11-25"},
		obtainMCPSpecVersionsFromRequest(nil, &[]string{"2026-07-28", "2025-11-25"}))
	assert.Equal(t, []string{"2025-06-18"}, obtainMCPSpecVersionsFromRequest(strPtr("2025-06-18"), nil))
	assert.Nil(t, obtainMCPSpecVersionsFromRequest(nil, nil))
}
