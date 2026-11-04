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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
)

// An in-cluster upstream is not public: a managed gateway takes it only for a project proxy's binding, whose
// upstream AMS resolved itself.
func TestUpstreamIsPublic_ProjectBindingPassesOnManagedGateway(t *testing.T) {
	inCluster := &models.UpstreamEndpoint{URL: "http://weather.dp-ns.svc.cluster.local:8080/mcp"}
	kindFor := func(projectScoped bool) *MCPProxyKind {
		return &MCPProxyKind{load: func(context.Context, models.ArtifactRef) (*mcpArtifact, error) {
			return &mcpArtifact{
				deployable: &models.MCPProxy{Configuration: models.MCPProxyConfig{
					Upstream: models.UpstreamConfig{Main: inCluster},
				}},
				projectScoped: projectScoped,
			}, nil
		}}
	}
	record := reconcile.Record{ArtifactRef: models.ArtifactRef{OUID: "org", ArtifactUUID: uuid.New()}}
	managed := &models.Gateway{UUID: uuid.New(), Hosting: models.GatewayHostingManaged}

	public, err := kindFor(true).upstreamIsPublic(context.Background(), record, managed)
	require.NoError(t, err)
	assert.True(t, public, "a project proxy's AMS-resolved upstream is placed on a managed gateway")

	public, err = kindFor(false).upstreamIsPublic(context.Background(), record, managed)
	require.NoError(t, err)
	assert.False(t, public, "an org proxy's private upstream still is not")
}
