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
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	bootstrapDir = "../ampinit/templates/thunder/bootstrap"

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

	diffScopeSets(t, "resource-server tree", catalogScopes(t), chartSide)
}
