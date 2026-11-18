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

package api

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

type fakeUI []modhost.ModuleUI

func (f fakeUI) UI() []modhost.ModuleUI { return f }

func TestUIManifestFiltersByPermission(t *testing.T) {
	src := fakeUI{{
		Name: "gateway", Version: "1.0.0",
		Pages: []modhost.UIEntry{
			{ID: "ai-gateway/gateways", Permission: "gateway:read"},
			{ID: "ai-gateway/external-agent"}, // also the core's: listed once
			{ID: "ai-gateway/llm-providers", Permission: rbac.AgentKindRead},
		},
		Nav:   []modhost.UIEntry{{ID: "gateways", Label: "Gateways", Permission: "gateway:read"}},
		Slots: []modhost.UIEntry{{ID: "agent.overview", Component: "Card", Permission: rbac.AgentKindRead}},
	}}
	req := httptest.NewRequest("GET", "/ui-manifest", nil)
	scopes := []string{rbac.AgentCreate.Scope(), "amp:gateway:read"}
	req = req.WithContext(jwtassertion.ContextWithScopeList(req.Context(), scopes))

	got := uiManifest(req, src)
	assert.Equal(t, []string{"ai-gateway/external-agent", "ai-gateway/gateways"}, got.Pages)
	assert.Len(t, got.Modules, 2)
	assert.Equal(t, "core", got.Modules[0].Name)
	assert.Equal(t, []string{"ai-gateway/external-agent"}, got.Modules[0].Pages)
	gw := got.Modules[1]
	assert.Equal(t, []string{"ai-gateway/gateways", "ai-gateway/external-agent"}, gw.Pages)
	assert.Len(t, gw.Nav, 1)
	assert.Empty(t, gw.Slots)
}

func TestUIManifestWithoutModules(t *testing.T) {
	req := httptest.NewRequest("GET", "/ui-manifest", nil)
	req = req.WithContext(jwtassertion.ContextWithScopeList(req.Context(), nil))
	got := uiManifest(req, nil)
	assert.Equal(t, []string{}, got.Pages)
	assert.Len(t, got.Modules, 1)
}

func TestUIManifestRootOUSeesEverything(t *testing.T) {
	src := fakeUI{{Name: "gateway", Pages: []modhost.UIEntry{{ID: "ai-gateway/gateways", Permission: "gateway:read"}}}}
	req := httptest.NewRequest("GET", "/ui-manifest", nil)
	ctx := jwtassertion.ContextWithScopeList(req.Context(), nil)
	ctx = jwtassertion.ContextWithTokenClaims(ctx, &jwtassertion.TokenClaims{OuHandle: config.GetConfig().RootOUHandle})
	got := uiManifest(req.WithContext(ctx), src)
	assert.Equal(t, []string{"ai-gateway/external-agent", "ai-gateway/gateways"}, got.Pages)

	ctx = jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{OuHandle: "default"})
	ctx = jwtassertion.ContextWithScopeList(ctx, nil)
	assert.Empty(t, uiManifest(req.WithContext(ctx), src).Pages)
}
