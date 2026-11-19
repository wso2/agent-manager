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
	"encoding/json"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// CoreUI is the console UI the core serves itself, in the console's
// always-on core module (acp-core): the external-agent page registers
// agents, which the core does.
var CoreUI = []modhost.UIEntry{
	{ID: "acp-core/external-agent", Permission: rbac.AgentCreate},
}

// UISource lists the console UI of the active modules.
type UISource interface {
	UI() []modhost.ModuleUI
}

func registerUIManifestRoute(rr *middleware.RouteRegistrar, src UISource) {
	rr.HandleFuncWithValidation("GET /ui-manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(uiManifest(r, src)); err != nil {
			logger.GetLogger(r.Context()).Error("failed to encode the UI manifest", "error", err)
		}
	})
}

func uiManifest(r *http.Request, src UISource) spec.UIManifestResponse {
	mods := []modhost.ModuleUI{{Name: "core", Pages: CoreUI}}
	if src != nil {
		mods = append(mods, src.UI()...)
	}
	// A root-OU token passes every permission check the routes that allow
	// it make (RequirePermissionAllowRootOU), so it sees every entry, the
	// same way the authorization middleware would treat it.
	claims := jwtassertion.GetTokenClaims(r.Context())
	rootOU := claims != nil && claims.OuHandle != "" && claims.OuHandle == config.GetConfig().RootOUHandle
	allowed := func(e modhost.UIEntry) bool {
		return rootOU || e.Permission == "" || jwtassertion.HoldsAnyScope(r.Context(), e.Permission)
	}
	out := spec.UIManifestResponse{Modules: []spec.UIManifestModule{}, Pages: []string{}}
	seen := map[string]bool{}
	for _, m := range mods {
		sm := spec.UIManifestModule{
			Name:  m.Name,
			Pages: []string{},
			Nav:   []spec.UIManifestNavItem{},
			Slots: []spec.UIManifestSlot{},
		}
		if m.Version != "" {
			sm.Version = &m.Version
		}
		for _, p := range m.Pages {
			if !allowed(p) {
				continue
			}
			sm.Pages = append(sm.Pages, p.ID)
			if !seen[p.ID] {
				seen[p.ID] = true
				out.Pages = append(out.Pages, p.ID)
			}
		}
		for _, n := range m.Nav {
			if allowed(n) {
				sm.Nav = append(sm.Nav, spec.UIManifestNavItem{Label: n.Label, Page: n.ID})
			}
		}
		for _, s := range m.Slots {
			if allowed(s) {
				sm.Slots = append(sm.Slots, spec.UIManifestSlot{Slot: s.ID, Component: s.Component})
			}
		}
		out.Modules = append(out.Modules, sm)
	}
	return out
}
