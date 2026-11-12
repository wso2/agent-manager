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

// Parity between the Go catalog and the Thunder bootstrap that enforces it.
//
// PredefinedRolePermissions has no consumers: it documents what the Thunder
// bootstrap resources actually enforce. Two independent copies of the same
// decision drift, and they had — agent:deploy-production was granted by the
// bootstrap and enforced by no route for as long as it existed. This test makes
// the Go declarations authoritative and the bootstrap checkable against them.
//
// The bootstrap resources are plain YAML files that deployments/standalone
// renders into ThunderID; only @@NAME@@ placeholders are substituted, and none
// of them appear in the documents read here.
package rbac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	bootstrapDir = "../../deployments/standalone/thunder/bootstrap"

	resourceServerDoc = "60-amp-resource-server.yaml"
	apiClientDoc      = "50-amp-api-client.yaml"
)

// chartRoleDocs maps each bootstrap role document to the Go role it must match.
var chartRoleDocs = map[string]string{
	"61-amp-role-admin.yaml":             RoleAdmin,
	"62-amp-role-developer.yaml":         RoleDeveloper,
	"63-amp-role-ai-lead.yaml":           RoleAILead,
	"64-amp-role-platform-engineer.yaml": RolePlatformEngineer,
}

// bootstrapDocument returns one bootstrap resource file.
func bootstrapDocument(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(bootstrapDir, name))
	if err != nil {
		t.Fatalf("read bootstrap resource %s: %v", name, err)
	}
	return raw
}

// diffScopeSets reports what one side has that the other does not, so a failure
// names the scopes rather than just the counts.
func diffScopeSets(t *testing.T, label string, goSide, chartSide map[Permission]bool) {
	t.Helper()
	for perm := range goSide {
		if !chartSide[perm] {
			t.Errorf("%s: Go declares %q but the chart does not", label, perm.Scope())
		}
	}
	for perm := range chartSide {
		if !goSide[perm] {
			t.Errorf("%s: the chart declares %q but Go does not", label, perm.Scope())
		}
	}
}

// TestResourceServerTreeMatchesCatalog is the load-bearing one. Thunder computes
// permission strings from this tree and silently drops any requested scope it
// cannot compute, so a scope missing here is a scope no token can ever carry —
// with no error anywhere.
func TestResourceServerTreeMatchesCatalog(t *testing.T) {
	var rs struct {
		Delimiter string `yaml:"delimiter"`
		Resources []struct {
			Handle  string `yaml:"handle"`
			Parent  string `yaml:"parent"`
			Actions []struct {
				Handle string `yaml:"handle"`
			} `yaml:"actions"`
		} `yaml:"resources"`
	}
	if err := yaml.Unmarshal(bootstrapDocument(t, resourceServerDoc), &rs); err != nil {
		t.Fatalf("parse %s: %v", resourceServerDoc, err)
	}
	if rs.Delimiter != ":" {
		t.Fatalf("resource server delimiter is %q, want \":\"; Permission.Scope() hard-codes the colon", rs.Delimiter)
	}

	chartSide := make(map[Permission]bool)
	for _, resource := range rs.Resources {
		// Only resources parented on the amp handle produce amp:<r>:<a> strings.
		if resource.Parent != ResourceServer {
			continue
		}
		for _, action := range resource.Actions {
			chartSide[Permission(resource.Handle+":"+action.Handle)] = true
		}
	}

	diffScopeSets(t, "resource-server tree", catalogScopes(t), chartSide)
}

// TestRoleDocumentsMatchPredefinedRoles keeps the documentation map and the
// enforcement in step. Either side changing alone is the drift this closes.
func TestRoleDocumentsMatchPredefinedRoles(t *testing.T) {
	for docKey, roleName := range chartRoleDocs {
		var doc struct {
			Name        string `yaml:"name"`
			Permissions []struct {
				ResourceServerID string   `yaml:"resourceServerId"`
				Permissions      []string `yaml:"permissions"`
			} `yaml:"permissions"`
		}
		if err := yaml.Unmarshal(bootstrapDocument(t, docKey), &doc); err != nil {
			t.Errorf("parse %s: %v", docKey, err)
			continue
		}
		if doc.Name != roleName {
			t.Errorf("%s declares name %q, want %q", docKey, doc.Name, roleName)
		}

		chartSide := make(map[Permission]bool)
		for _, block := range doc.Permissions {
			// MCP resource servers repeat a subset of the same scopes; the
			// canonical grant is the amp-resource-server block.
			if block.ResourceServerID != "amp-resource-server" {
				continue
			}
			for _, scope := range block.Permissions {
				trimmed, ok := strings.CutPrefix(scope, ResourceServer+":")
				if !ok {
					t.Errorf("%s: scope %q is not prefixed %q", docKey, scope, ResourceServer+":")
					continue
				}
				chartSide[Permission(trimmed)] = true
			}
		}

		goSide := make(map[Permission]bool)
		for _, perm := range PredefinedRolePermissions[roleName] {
			goSide[perm] = true
		}
		diffScopeSets(t, docKey, goSide, chartSide)
	}
}

// TestAmpScopesAllowlistCoversCatalog guards the third failure mode. Thunder
// grants only scopes an app's registered list contains, so a catalog scope
// missing from the API client's scopes is a role binding that resolves to
// nothing at token time.
func TestAmpScopesAllowlistCoversCatalog(t *testing.T) {
	chartSide := make(map[Permission]bool)
	for _, scope := range apiClientScopes(t) {
		if trimmed, ok := strings.CutPrefix(scope, ResourceServer+":"); ok {
			chartSide[Permission(trimmed)] = true
		}
	}

	diffScopeSets(t, "API client scopes", catalogScopes(t), chartSide)
}

// apiClientScopes returns the scopes registered on the bootstrap's
// amp-api-client application.
func apiClientScopes(t *testing.T) []string {
	t.Helper()
	var app struct {
		InboundAuthConfig []struct {
			Config struct {
				Scopes []string `yaml:"scopes"`
			} `yaml:"config"`
		} `yaml:"inboundAuthConfig"`
	}
	if err := yaml.Unmarshal(bootstrapDocument(t, apiClientDoc), &app); err != nil {
		t.Fatalf("parse %s: %v", apiClientDoc, err)
	}
	for _, cfg := range app.InboundAuthConfig {
		if len(cfg.Config.Scopes) > 0 {
			return cfg.Config.Scopes
		}
	}
	t.Fatalf("%s registers no scopes", apiClientDoc)
	return nil
}
