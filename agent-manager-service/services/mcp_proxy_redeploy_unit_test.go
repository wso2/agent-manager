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
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// A redeploy renders every artifact before queueing any: one that fails leaves the others' gateways untouched.
func TestRedeployMCPProxyQueuesNothingWhenAnArtifactFailsToRender(t *testing.T) {
	healthy, broken := uuid.New(), uuid.New()
	loadErr := errors.New("connection reset")
	kind := &MCPProxyKind{load: func(_ context.Context, artifact models.ArtifactRef) (*mcpArtifact, error) {
		if artifact.ArtifactUUID == broken {
			return nil, loadErr
		}
		return &mcpArtifact{deployable: &models.MCPProxy{Handle: "proxy", Configuration: models.MCPProxyConfig{
			Upstream: models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: "https://mcp.example.com"}},
		}}, proxyHandle: "proxy"}, nil
	}}
	// QueueRenderFunc is nil: queueing a redeploy panics.
	core := reconcile.NewCore(nil, &repomocks.DeploymentRepositoryMock{}, nil, reconcile.Kinds{kind}, reconcile.Config{},
		slog.New(slog.DiscardHandler))
	svc := NewMCPProxyService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler), nil,
		nil, nil, core, kind)
	proxy := &models.MCPProxy{Endpoints: []models.MCPProxyEndpoint{{
		Environments: []models.MCPProxyEndpointEnvironment{{ArtifactUUID: healthy}, {ArtifactUUID: broken}},
	}}}

	err := svc.RedeployMCPProxy(context.Background(), proxy, "ou-1")

	assert.ErrorIs(t, err, loadErr)
}
