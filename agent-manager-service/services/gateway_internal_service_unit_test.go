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
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type activeDeploymentLookup func(s *GatewayInternalAPIService, ctx context.Context, id, ouID, gatewayID string) (map[string]string, error)

// Both artifact kinds share one lookup; each keeps its own not-found sentinel.
var activeDeploymentLookups = []struct {
	name        string
	lookup      activeDeploymentLookup
	notFound    error
	notNotFound error
}{
	{
		name:        "agent",
		lookup:      (*GatewayInternalAPIService).GetActiveAgentDeploymentByGateway,
		notFound:    utils.ErrAgentArtifactNotFound,
		notNotFound: utils.ErrMCPProxyNotFound,
	},
	{
		name:        "mcp proxy",
		lookup:      (*GatewayInternalAPIService).GetActiveMCPProxyDeploymentByGateway,
		notFound:    utils.ErrMCPProxyNotFound,
		notNotFound: utils.ErrAgentArtifactNotFound,
	},
	{
		name:        "rest api",
		lookup:      (*GatewayInternalAPIService).GetActiveRestAPIDeploymentByGateway,
		notFound:    utils.ErrAPINotFound,
		notNotFound: utils.ErrAgentArtifactNotFound,
	},
}

const testArtifactID = "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f"

func newGatewayInternalServiceWithDeployments(repo *repomocks.DeploymentRepositoryMock) *GatewayInternalAPIService {
	return NewGatewayInternalAPIService(nil, nil, repo, nil, nil, nil, nil, gatewayKinds())
}

// gatewayKinds names artifacts as the registered kinds do; naming needs none of their dependencies.
func gatewayKinds() *reconcile.Core {
	return reconcile.NewCore(nil, nil, nil, reconcile.Kinds{NewLLMProviderKind(), &LLMProxyKind{}, &MCPProxyKind{}, &A2AAgentKind{}},
		reconcile.Config{}, discardLogger())
}

func TestActiveArtifactDeploymentByGatewayReturnsYamlKeyedByID(t *testing.T) {
	for _, tt := range activeDeploymentLookups {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.DeploymentRepositoryMock{
				GetPullableFunc: func(_ context.Context, ouID string, artifactUUID uuid.UUID, gatewayID string) (*models.Deployment, error) {
					assert.Equal(t, testArtifactID, artifactUUID.String())
					assert.Equal(t, "gw-1", gatewayID)
					assert.Equal(t, "ou-1", ouID)
					return &models.Deployment{Content: []byte("kind: Agent\n")}, nil
				},
			}
			got, err := tt.lookup(newGatewayInternalServiceWithDeployments(repo), context.Background(), testArtifactID, "ou-1", "gw-1")
			require.NoError(t, err)
			assert.Equal(t, map[string]string{testArtifactID: "kind: Agent\n"}, got)
		})
	}
}

func TestActiveArtifactDeploymentByGatewayNotDeployed(t *testing.T) {
	for _, tt := range activeDeploymentLookups {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.DeploymentRepositoryMock{
				GetPullableFunc: func(context.Context, string, uuid.UUID, string) (*models.Deployment, error) {
					return nil, nil //nolint:nilnil // the repository reports "no current deployment" this way
				},
			}
			_, err := tt.lookup(newGatewayInternalServiceWithDeployments(repo), context.Background(), testArtifactID, "ou-1", "gw-1")
			assert.ErrorIs(t, err, tt.notFound)
			assert.NotErrorIs(t, err, tt.notNotFound)
			// The gateway controllers rely on this: they map only the kind's own sentinel to 404.
			assert.NotErrorIs(t, err, utils.ErrDeploymentNotActive)
		})
	}
}

// The gateway names these artifacts by UUID; anything else can't be one of ours.
func TestActiveArtifactDeploymentByGatewayRejectsANonUUIDID(t *testing.T) {
	for _, tt := range activeDeploymentLookups {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.lookup(newGatewayInternalServiceWithDeployments(&repomocks.DeploymentRepositoryMock{}),
				context.Background(), "not-a-uuid", "ou-1", "gw-1")
			assert.ErrorIs(t, err, tt.notFound)
		})
	}
}

func TestActiveArtifactDeploymentByGatewayRepoErrorIsNotNotFound(t *testing.T) {
	repoErr := errors.New("db down")
	for _, tt := range activeDeploymentLookups {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.DeploymentRepositoryMock{
				GetPullableFunc: func(context.Context, string, uuid.UUID, string) (*models.Deployment, error) {
					return nil, repoErr
				},
			}
			_, err := tt.lookup(newGatewayInternalServiceWithDeployments(repo), context.Background(), testArtifactID, "ou-1", "gw-1")
			assert.ErrorIs(t, err, repoErr)
			assert.NotErrorIs(t, err, tt.notFound)
		})
	}
}

// The stored Agent YAML holds the proxy's upstream secret only as secretRef; the gateway is served the value.
func TestActiveAgentDeploymentDecryptsTheUpstreamSecret(t *testing.T) {
	key := make([]byte, 32)
	ciphertext, err := utils.EncryptBytes([]byte("s3cret"), key)
	require.NoError(t, err)
	in := proxiedA2AInput()
	secretRef := base64.StdEncoding.EncodeToString(ciphertext)
	in.UpstreamAuth.SecretRef = &secretRef
	stored, err := generateA2AAgentDeploymentYAML(in)
	require.NoError(t, err)
	repo := &repomocks.DeploymentRepositoryMock{
		GetPullableFunc: func(context.Context, string, uuid.UUID, string) (*models.Deployment, error) {
			return &models.Deployment{Content: []byte(stored)}, nil
		},
	}
	svc := NewGatewayInternalAPIService(nil, nil, repo, nil, nil, nil, key, gatewayKinds())

	got, err := svc.GetActiveAgentDeploymentByGateway(context.Background(), testArtifactID, "ou-1", "gw-1")
	require.NoError(t, err)

	var doc struct {
		Spec struct {
			Upstream struct {
				Auth map[string]interface{} `yaml:"auth"`
			} `yaml:"upstream"`
		} `yaml:"spec"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got[testArtifactID]), &doc))
	assert.Equal(t, "s3cret", doc.Spec.Upstream.Auth["value"])
	assert.Equal(t, "X-Upstream-Key", doc.Spec.Upstream.Auth["header"])
	assert.NotContains(t, doc.Spec.Upstream.Auth, "secretRef")
}

func TestActiveArtifactDeploymentByGatewayUnparseableYaml(t *testing.T) {
	for _, tt := range activeDeploymentLookups {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.DeploymentRepositoryMock{
				GetPullableFunc: func(context.Context, string, uuid.UUID, string) (*models.Deployment, error) {
					return &models.Deployment{Content: []byte("key: [unclosed")}, nil
				},
			}
			_, err := tt.lookup(newGatewayInternalServiceWithDeployments(repo), context.Background(), testArtifactID, "ou-1", "gw-1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to resolve secrets")
			assert.NotErrorIs(t, err, tt.notFound)
		})
	}
}
