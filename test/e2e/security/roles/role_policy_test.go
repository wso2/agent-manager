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

package roles

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

type roleProbe struct {
	role    string
	scope   string
	allowed bool
	method  string
	path    func(*framework.Config) string
}

const absentResourceName = framework.E2EProjectPrefix + "sec-absent"

var _ = Describe("SEC-ROLE-001: deployed role policy", Label("security", "roles"), func() {
	for _, roleName := range roleNames {
		roleName := roleName
		It("issues exactly the deployed permissions to the "+roleName+" persona", func() {
			persona := personas[roleName]
			issued, err := framework.TokenScopes(persona.Token)
			Expect(err).NotTo(HaveOccurred())

			var ampScopes []string
			for scope := range issued {
				if strings.HasPrefix(scope, "amp:") {
					ampScopes = append(ampScopes, scope)
				}
			}
			expected := append([]string(nil), persona.RolePermissions...)
			sort.Strings(ampScopes)
			sort.Strings(expected)
			Expect(ampScopes).To(Equal(expected),
				"Thunder token scopes differ from the permissions configured on role %s", roleName)
		})
	}

	for _, probe := range roleProbes() {
		probe := probe
		verb := "denies"
		if probe.allowed {
			verb = "allows"
		}
		It(fmt.Sprintf("%s %s for %s", verb, probe.scope, probe.role), func(ctx SpecContext) {
			persona := personas[probe.role]
			issued, err := framework.TokenScopes(persona.Token)
			Expect(err).NotTo(HaveOccurred())
			if probe.allowed {
				Expect(issued).To(HaveKey(probe.scope), "role policy no longer grants the positive-control scope")
			} else {
				Expect(issued).NotTo(HaveKey(probe.scope), "role policy unexpectedly grants the denied scope")
			}

			client := framework.NewAMPClientWithToken(cfg, persona.Token)
			resp, err := client.DoWithContext(ctx, probe.method, probe.path(cfg), nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			label := fmt.Sprintf("%s acting on %s", probe.role, probe.scope)
			if probe.allowed {
				framework.ExpectNotForbidden(Default, resp, label)
			} else {
				framework.ExpectForbidden(Default, resp, label)
			}
		})
	}
})

func roleProbes() []roleProbe {
	orgPath := func(suffix string) func(*framework.Config) string {
		return func(cfg *framework.Config) string {
			return "/api/v1/orgs/" + cfg.DefaultOrg + suffix
		}
	}
	// The AI Gateway module's API.
	gwOrgPath := func(suffix string) func(*framework.Config) string {
		return func(cfg *framework.Config) string {
			return "/m/ai-gateway/orgs/" + cfg.DefaultOrg + suffix
		}
	}
	return []roleProbe{
		// Developer: owns project and agent development, not platform or IAM.
		{"developer", "amp:project:create", true, http.MethodPost, orgPath("/projects")},
		{"developer", "amp:environment:update", false, http.MethodPut, orgPath("/environments/" + absentResourceName)},
		{"developer", "amp:role:update", false, http.MethodPut, orgPath("/identities/roles/" + absentResourceName)},

		// AI Lead: owns AI providers, not project/platform administration.
		{"ai-lead", "amp:ai-gateway:llm-providers.create", true, http.MethodPost, gwOrgPath("/llm-providers")},
		{"ai-lead", "amp:project:create", false, http.MethodPost, orgPath("/projects")},
		{"ai-lead", "amp:environment:update", false, http.MethodPut, orgPath("/environments/" + absentResourceName)},

		// Platform Engineer: owns infrastructure and production delivery, not agent authoring or IAM.
		{"platform-engineer", "amp:environment:update", true, http.MethodPut, orgPath("/environments/" + absentResourceName)},
		{"platform-engineer", "amp:ai-gateway:gateways.create", true, http.MethodPost, gwOrgPath("/gateways")},
		{"platform-engineer", "amp:agent:create", false, http.MethodPost, orgPath("/projects/" + absentResourceName + "/agents")},
		{"platform-engineer", "amp:role:update", false, http.MethodPut, orgPath("/identities/roles/" + absentResourceName)},

		// Admin positive controls cover the privilege-granting IAM boundary.
		{"admin", "amp:role:update", true, http.MethodPut, orgPath("/identities/roles/" + absentResourceName)},
		{"admin", "amp:org:invite-member", true, http.MethodPost, orgPath("/identities/users/invite")},
	}
}
