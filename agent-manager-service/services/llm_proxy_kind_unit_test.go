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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// goldenProxy is an agent's proxy in front of goldenProvider, with every part of the proxy render: api-key
// security, upstream auth, resilience and a user policy.
func goldenProxy() *models.LLMProxy {
	enabled := true
	ctxPath, vhost, timeout := "/agent-proxy", "proxy.example.com", "10s"
	authType, authHeader, ref := "api-key", "X-Upstream-Key", "proxy-upstream-key"
	return &models.LLMProxy{
		UUID:   uuid.MustParse("7a2d3b63-1e8f-4d2b-8b4f-1f6c7a8b9cad"),
		Handle: "acme-agent-proxy",
		Configuration: models.LLMProxyConfig{
			Name: "Acme Agent Proxy", Version: "v1.0", Context: &ctxPath, Vhost: &vhost,
			Provider:     goldenProvider().UUID.String(),
			UpstreamAuth: &models.UpstreamAuth{Type: &authType, Header: &authHeader, SecretRef: &ref},
			Resilience:   &models.Resilience{Timeout: &timeout},
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey:  &models.APIKeySecurity{Enabled: &enabled, Key: "X-Agent-Key", In: "Header"},
			},
			Policies: []models.LLMPolicy{{Name: "content-length-guardrail", Version: "v0.3.1", Paths: []models.LLMPolicyPath{
				{Path: "/*", Methods: []string{"*"}, Params: map[string]any{"maxRequestBytes": 4096}},
			}}},
		},
	}
}

func goldenProxyKind(proxy *models.LLMProxy) *LLMProxyKind {
	return &LLMProxyKind{
		load: func(context.Context, models.ArtifactRef) (*models.LLMProxy, *models.LLMProvider, error) {
			return proxy, goldenProvider(), nil
		},
		vhostOf: func(*gorm.DB, models.ArtifactRef) string { return proxyVhost(proxy) },
	}
}

func TestLLMProxyKindRendersTodaysYAML(t *testing.T) {
	proxy := goldenProxy()
	ref := models.ArtifactRef{OUID: "ou-1", ArtifactUUID: proxy.UUID, Kind: models.KindLLMProxy, Handle: proxy.Handle}

	rendered, err := goldenProxyKind(proxy).Render(context.Background(), ref)

	require.NoError(t, err)
	assert.Equal(t, readGolden(t, "llm_proxy.yaml"), string(rendered.Content))
	assert.Regexp(t, `^acme-agent-proxy-[0-9a-f]{8}$`, rendered.Name)
}

// An invalid security config is a validation error the save returns as a 4xx, and retrying never fixes it.
func TestLLMProxyKindRejectsAnInvalidSecurityConfig(t *testing.T) {
	proxy := goldenProxy()
	proxy.Configuration.Security.APIKey.Key = ""

	_, err := goldenProxyKind(proxy).Render(context.Background(), models.ArtifactRef{Handle: proxy.Handle})

	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.True(t, reconcile.IsPermanent(err))
}

func TestLLMProxyKindEvents(t *testing.T) {
	proxy := goldenProxy()
	kind := goldenProxyKind(proxy)
	ref := models.ArtifactRef{OUID: "ou-1", ArtifactUUID: proxy.UUID, Kind: models.KindLLMProxy, Handle: proxy.Handle}
	p := models.Placement{ArtifactRef: ref, GatewayUUID: uuid.New()}
	at := models.NewPerformedAt(time.Now())

	assert.Equal(t, proxy.Handle, kind.GatewayArtifactID(ref), "gateways store proxies under their handle")

	deploy := kind.DeployEvent(reconcile.EventRef{Placement: p, DeploymentID: "d-1", PerformedAt: at})
	assert.Equal(t, "llmproxy.deployed", string(deploy.EventType))
	assert.Equal(t, "CREATE", deploy.Action)
	assert.Equal(t, proxy.Handle, deploy.EntityID)
	assert.Equal(t, map[string]any{
		"proxyId": proxy.Handle, "deploymentId": "d-1", "performedAt": at.Format(time.RFC3339Nano),
		"vhost": "proxy.example.com", "environment": "production", "gatewayId": p.GatewayUUID.String(),
		"organizationId": "ou-1", "status": "DEPLOYED",
	}, eventPayload(t, deploy.EventData))

	undeploy := kind.UndeployEvent(reconcile.EventRef{Placement: p, PerformedAt: at})
	assert.Equal(t, "llmproxy.undeployed", string(undeploy.EventType))
	assert.Equal(t, "DELETE", undeploy.Action)
	assert.Equal(t, map[string]any{
		"proxyId": proxy.Handle, "deploymentId": "", "performedAt": at.Format(time.RFC3339Nano),
		"vhost": "proxy.example.com", "environment": "production", "gatewayId": p.GatewayUUID.String(),
		"organizationId": "ou-1",
	}, eventPayload(t, undeploy.EventData))

	deleted := kind.DeleteEvent(ref)
	assert.Equal(t, "llmproxy.deleted", string(deleted.EventType))
	assert.Equal(t, map[string]any{"proxyId": proxy.Handle}, eventPayload(t, deleted.EventData))
}
