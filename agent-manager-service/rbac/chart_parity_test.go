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

// Parity between the Go catalog and the Helm chart that enforces it.
//
// PredefinedRolePermissions has no consumers: it documents what the Thunder
// bootstrap ConfigMap actually enforces. Two independent copies of the same
// decision drift, and they had — agent:deploy-production was granted by the
// chart and enforced by no route for as long as it existed. This test makes the
// Go declarations authoritative and the chart checkable against them.
//
// The four role documents and the resource-server document inside the ConfigMap
// are plain YAML with no Go templating, so they parse directly. If templating is
// ever introduced into them, this test breaks loudly rather than silently
// stopping checking — which is the correct failure.
package rbac

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	bootstrapPath = "../../deployments/helm-charts/wso2-amp-thunder-extension/templates/amp-thunder-bootstrap.yaml"
	extValuesPath = "../../deployments/helm-charts/wso2-amp-thunder-extension/values.yaml"
	// The catalog is advertised a second time, from the agent-manager chart, as
	// OAUTH_SCOPES_SUPPORTED.
	amChartValuesPath = "../../deployments/helm-charts/wso2-agent-manager/values.yaml"

	resourceServerDoc = "60-amp-resource-server.yaml"
)

// chartRoleDocs maps each bootstrap role document to the Go role it must match.
var chartRoleDocs = map[string]string{
	"61-amp-role-admin.yaml":             RoleAdmin,
	"62-amp-role-developer.yaml":         RoleDeveloper,
	"63-amp-role-ai-lead.yaml":           RoleAILead,
	"64-amp-role-platform-engineer.yaml": RolePlatformEngineer,
}

// configMapKeyLine matches the start of any ConfigMap data entry, which is what
// bounds the document extracted before it.
var configMapKeyLine = regexp.MustCompile(`^  [\w.-]+: \|\s*$`)

// bootstrapDocument returns one ConfigMap literal block, dedented so it parses
// as YAML on its own.
func bootstrapDocument(t *testing.T, raw, key string) string {
	t.Helper()
	lines := strings.Split(raw, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == key+": |" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("ConfigMap key %q not found in %s", key, bootstrapPath)
	}
	var body []string
	templateDepth := 0
	for _, line := range lines[start:] {
		if configMapKeyLine.MatchString(line) {
			break
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{{- range ") || strings.HasPrefix(trimmed, "{{- if ") {
			templateDepth++
			continue
		}
		if strings.HasPrefix(trimmed, "{{- end") && templateDepth > 0 {
			templateDepth--
			continue
		}
		if templateDepth > 0 {
			continue
		}
		if strings.Contains(line, "{{") {
			t.Fatalf("document %q now contains Go templating (%q); this test can no longer parse it as YAML", key, strings.TrimSpace(line))
		}
		body = append(body, strings.TrimPrefix(line, "    "))
	}
	return strings.Join(body, "\n")
}

func readBootstrap(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(bootstrapPath)
	if err != nil {
		t.Skipf("chart not present at %s (module checked out without the chart?): %v", bootstrapPath, err)
	}
	return string(raw)
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
	if err := yaml.Unmarshal([]byte(bootstrapDocument(t, readBootstrap(t), resourceServerDoc)), &rs); err != nil {
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
	raw := readBootstrap(t)
	for docKey, roleName := range chartRoleDocs {
		var doc struct {
			Name        string `yaml:"name"`
			Permissions []struct {
				ResourceServerID string   `yaml:"resourceServerId"`
				Permissions      []string `yaml:"permissions"`
			} `yaml:"permissions"`
		}
		if err := yaml.Unmarshal([]byte(bootstrapDocument(t, raw, docKey)), &doc); err != nil {
			t.Errorf("parse %s: %v", docKey, err)
			continue
		}
		if doc.Name != roleName {
			t.Errorf("%s declares name %q, want %q", docKey, doc.Name, roleName)
		}

		chartSide := make(map[Permission]bool)
		for _, block := range doc.Permissions {
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
// missing from ampScopes is a role binding that resolves to nothing at token
// time.
func TestAmpScopesAllowlistCoversCatalog(t *testing.T) {
	raw, err := os.ReadFile(extValuesPath)
	if err != nil {
		t.Skipf("chart not present at %s: %v", extValuesPath, err)
	}
	var values struct {
		Thunder struct {
			Bootstrap struct {
				AmpScopes []string `yaml:"ampScopes"`
			} `yaml:"bootstrap"`
		} `yaml:"thunder"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("parse %s: %v", extValuesPath, err)
	}

	chartSide := make(map[Permission]bool)
	for _, scope := range values.Thunder.Bootstrap.AmpScopes {
		if trimmed, ok := strings.CutPrefix(scope, ResourceServer+":"); ok {
			chartSide[Permission(trimmed)] = true
		}
	}

	diffScopeSets(t, "ampScopes allowlist", catalogScopes(t), chartSide)
}

// TestAdvertisedScopesMatchCatalog covers the one copy of the catalog that lives
// outside the Thunder extension chart: OAUTH_SCOPES_SUPPORTED, which
// agent-manager-service publishes in its RFC 9728 protected-resource metadata
// (api/well_known_routes.go) and which amctl reads to decide which scopes to
// request at login (cli/pkg/clients/discovery.go).
//
// A scope missing here is one the CLI never asks for, so an operator holding the
// role still gets a token without it — a 403 on a route their role allows, with
// nothing wrong anywhere the error points. Unlike the resource-server tree this
// is a single comma-separated string in a different chart, so nothing else
// checks it.
func TestAdvertisedScopesMatchCatalog(t *testing.T) {
	raw, err := os.ReadFile(amChartValuesPath)
	if err != nil {
		t.Skipf("chart not present at %s: %v", amChartValuesPath, err)
	}
	var values struct {
		AgentManagerService struct {
			Config struct {
				OAuthScopesSupported string `yaml:"oauthScopesSupported"`
			} `yaml:"config"`
		} `yaml:"agentManagerService"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("parse %s: %v", amChartValuesPath, err)
	}

	advertised := values.AgentManagerService.Config.OAuthScopesSupported
	if strings.TrimSpace(advertised) == "" {
		t.Fatalf("%s: agentManagerService.config.oauthScopesSupported is empty; "+
			"the protected-resource metadata endpoint answers 503 without it", amChartValuesPath)
	}

	chartSide := make(map[Permission]bool)
	for _, scope := range strings.Split(advertised, ",") {
		if trimmed, ok := strings.CutPrefix(strings.TrimSpace(scope), ResourceServer+":"); ok {
			chartSide[Permission(trimmed)] = true
		}
	}

	diffScopeSets(t, "oauthScopesSupported", catalogScopes(t), chartSide)
}
