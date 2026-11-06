//
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

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func annotatedExternalSecret(name string, annotations map[string]interface{}) gen.ResourceNode {
	meta := map[string]interface{}{"name": name}
	if annotations != nil {
		meta["annotations"] = annotations
	}
	return gen.ResourceNode{Kind: "ExternalSecret", Name: name, Object: map[string]interface{}{"metadata": meta}}
}

func TestForceSyncRevisionApplied(t *testing.T) {
	tree := func(nodes ...gen.ResourceNode) *gen.K8sResourceTreeResponse {
		return &gen.K8sResourceTreeResponse{RenderedReleases: []gen.ReleaseResourceTree{{Nodes: nodes}}}
	}
	deployment := gen.ResourceNode{Kind: "Deployment", Name: "d", Object: map[string]interface{}{
		"metadata": map[string]interface{}{"annotations": map[string]interface{}{"force-sync": "42"}},
	}}

	assert.True(t, forceSyncRevisionApplied(tree(annotatedExternalSecret("a", map[string]interface{}{"force-sync": "42"})), "42"))
	assert.True(t, forceSyncRevisionApplied(tree(
		annotatedExternalSecret("a", map[string]interface{}{"force-sync": "42"}),
		annotatedExternalSecret("file", nil),
	), "42"), "an ExternalSecret without the annotation (a file secret) must not block")
	assert.False(t, forceSyncRevisionApplied(tree(annotatedExternalSecret("a", map[string]interface{}{"force-sync": "41"})), "42"), "old revision still live")
	assert.False(t, forceSyncRevisionApplied(tree(
		annotatedExternalSecret("a", map[string]interface{}{"force-sync": "42"}),
		annotatedExternalSecret("b", map[string]interface{}{"force-sync": "41"}),
	), "42"), "any annotated ExternalSecret on another revision is not yet applied")
	assert.False(t, forceSyncRevisionApplied(tree(annotatedExternalSecret("a", nil)), "42"), "no annotation, e.g. old ComponentType")
	assert.False(t, forceSyncRevisionApplied(tree(deployment), "42"), "only ExternalSecrets count")
	assert.False(t, forceSyncRevisionApplied(nil, "42"))
	assert.False(t, forceSyncRevisionApplied(tree(annotatedExternalSecret("a", map[string]interface{}{"force-sync": ""})), ""))
}
