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
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
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
				projectScoped: projectScoped, resolvedUpstream: projectScoped,
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

// Placement never takes a self-hosted gateway for a project proxy's binding, even one it is already anchored on;
// an org proxy keeps using its self-hosted anchor.
func TestTargets_ProjectBindingSkipsSelfHostedGateway(t *testing.T) {
	env := uuid.New()
	selfHosted := &models.Gateway{
		UUID: uuid.New(), Hosting: models.GatewayHostingSelfHosted, GatewayFunctionalityType: models.GatewayRoleBoth,
	}
	kindFor := func(projectScoped bool) *MCPProxyKind {
		return &MCPProxyKind{
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
					return []string{selfHosted.UUID.String()}, nil
				},
			},
			gatewayRepo: &repomocks.GatewayRepositoryMock{
				EnvironmentMappingExistsFunc: func(string, string) (bool, error) { return true, nil },
				GetByUUIDFunc:                func(string) (*models.Gateway, error) { return selfHosted, nil },
			},
			load: func(context.Context, models.ArtifactRef) (*mcpArtifact, error) {
				return &mcpArtifact{deployable: &models.MCPProxy{}, projectScoped: projectScoped}, nil
			},
		}
	}
	record := reconcile.Record{
		ArtifactRef:     models.ArtifactRef{OUID: "org", ArtifactUUID: uuid.New()},
		EnvironmentUUID: env,
	}

	_, ok, err := kindFor(true).Targets(context.Background(), record)
	require.NoError(t, err)
	assert.False(t, ok, "a project binding is not placed on a self-hosted gateway")

	gateway, ok, err := kindFor(false).Targets(context.Background(), record)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, selfHosted.UUID, gateway, "an org proxy keeps its self-hosted anchor")
}

// A project proxy registered by URL gets no pass: its URL is the caller's, checked as an org proxy's is.
func TestUpstreamIsPublic_URLProjectBindingIsChecked(t *testing.T) {
	kind := &MCPProxyKind{load: func(context.Context, models.ArtifactRef) (*mcpArtifact, error) {
		return &mcpArtifact{
			deployable: &models.MCPProxy{Configuration: models.MCPProxyConfig{
				Upstream: models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: "http://10.1.2.3/mcp"}},
			}},
			projectScoped: true,
		}, nil
	}}
	record := reconcile.Record{ArtifactRef: models.ArtifactRef{OUID: "org", ArtifactUUID: uuid.New()}}
	managed := &models.Gateway{UUID: uuid.New(), Hosting: models.GatewayHostingManaged}

	public, err := kind.upstreamIsPublic(context.Background(), record, managed)
	require.NoError(t, err)
	assert.False(t, public)
}
