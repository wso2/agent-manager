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
// The ampinit package generates the bootstrap: role grants come from
// PredefinedRolePermissions and client scope lists from the resource-server
// tree, so the tree is the one hand-written copy left to check. Its template
// has no template actions, so it is read here as plain YAML (ampinit imports
// this package, so it cannot be rendered from here).
package rbac

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	bootstrapDir = "../ampinit/templates/thunder/bootstrap"
	// The catalog is advertised a second time, as OAUTH_SCOPES_SUPPORTED on the
	// amp-api service of the standalone stack. It used to live in the
	// agent-manager Helm chart; the charts went with the OpenChoreo removal and
	// the Compose stack is now the only place that declares it.
	standaloneComposePath = "../../deployments/standalone/docker-compose.yml"

	resourceServerDoc = "60-amp-resource-server.yaml"
)

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

	diffScopeSets(t, "resource-server tree", catalogScopes(t), withoutModuleOwned(chartSide))
}

// TestAdvertisedScopesMatchCatalog covers the second copy of the catalog:
// OAUTH_SCOPES_SUPPORTED, which agent-manager-service publishes in its RFC 9728
// protected-resource metadata (api/well_known_routes.go) and which amctl reads
// to decide which scopes to request at login (cli/pkg/clients/discovery.go).
//
// A scope missing there is one the CLI never asks for, so an operator holding
// the role still gets a token without it — a 403 on a route their role allows,
// with nothing wrong anywhere the error points. It is a single comma-separated
// string in a deployment file, so nothing else checks it.
func TestAdvertisedScopesMatchCatalog(t *testing.T) {
	raw, err := os.ReadFile(standaloneComposePath)
	if err != nil {
		t.Skipf("standalone stack not present at %s: %v", standaloneComposePath, err)
	}

	var compose struct {
		Services map[string]struct {
			Environment composeEnv `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &compose); err != nil {
		t.Fatalf("parse %s: %v", standaloneComposePath, err)
	}

	advertised := compose.Services["amp-api"].Environment["OAUTH_SCOPES_SUPPORTED"]
	if strings.TrimSpace(advertised) == "" {
		// Unset, the API advertises the whole catalog (api.catalogScopes).
		t.Skipf("%s: amp-api leaves OAUTH_SCOPES_SUPPORTED to the API's default, the catalog", standaloneComposePath)
	}

	chartSide := make(map[Permission]bool)
	for _, scope := range strings.Split(advertised, ",") {
		if trimmed, ok := strings.CutPrefix(strings.TrimSpace(scope), ResourceServer+":"); ok {
			chartSide[Permission(trimmed)] = true
		}
	}

	diffScopeSets(t, "OAUTH_SCOPES_SUPPORTED", catalogScopes(t), withoutModuleOwned(chartSide))
}

// composeEnv is a service's environment in either form Compose takes: a map,
// or a list of KEY=VALUE, where a bare KEY has no value.
type composeEnv map[string]string

func (e *composeEnv) UnmarshalYAML(n *yaml.Node) error {
	out := composeEnv{}
	switch n.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := n.Decode(&m); err != nil {
			return err
		}
		for k, v := range m {
			out[k] = v
		}
	case yaml.SequenceNode:
		var items []string
		if err := n.Decode(&items); err != nil {
			return err
		}
		for _, item := range items {
			k, v, _ := strings.Cut(item, "=")
			out[k] = v
		}
	default:
		return fmt.Errorf("environment: want a map or a list, got %s", n.Tag)
	}
	*e = out
	return nil
}

// distributionModules are the modules the full distribution's configuration
// advertises scopes for; their manifests, not this package, declare them.
var distributionModules = []string{"ai-gateway"}

// withoutModuleOwned drops the permissions of the distribution's modules.
func withoutModuleOwned(chartSide map[Permission]bool) map[Permission]bool {
	out := make(map[Permission]bool, len(chartSide))
	for perm := range chartSide {
		resource, _, _ := strings.Cut(string(perm), ":")
		if !slices.Contains(distributionModules, resource) {
			out[perm] = true
		}
	}
	return out
}

func TestComposeEnvReadsBothForms(t *testing.T) {
	for name, doc := range map[string]string{
		"map":  "environment:\n  A: x=y\n  B:\n",
		"list": "environment:\n  - A=x=y\n  - B\n",
	} {
		var svc struct {
			Environment composeEnv `yaml:"environment"`
		}
		if err := yaml.Unmarshal([]byte(doc), &svc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := svc.Environment["A"]; got != "x=y" {
			t.Errorf("%s: A = %q, want %q", name, got, "x=y")
		}
		if v, ok := svc.Environment["B"]; !ok || v != "" {
			t.Errorf("%s: B = %q (present %v), want empty and present", name, v, ok)
		}
	}
}
