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
	"os"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func TestBuildTraitAddsArtifactIDForAPIManagement(t *testing.T) {
	uid := "8af36e12-6df2-4c58-b64f-d70fc8d4fecd"
	ocClient := &openChoreoClient{}

	trait, err := ocClient.buildTrait(
		t.Context(),
		"default",
		"project",
		"travel-agent",
		&gen.Component{Metadata: gen.ObjectMeta{Name: "travel-agent", Uid: &uid}},
		TraitRequest{TraitKind: TraitKindTrait, TraitType: TraitAPIManagement},
	)
	if err != nil {
		t.Fatalf("buildTrait returned error: %v", err)
	}
	if trait.Parameters == nil {
		t.Fatalf("expected trait parameters")
	}
	if got := (*trait.Parameters)["artifactID"]; got != uid {
		t.Fatalf("expected artifactID %q, got %#v", uid, got)
	}
}

func TestBuildTraitAddsAPIKeyAuthPolicyForAPIManagement(t *testing.T) {
	ocClient := &openChoreoClient{}

	trait, err := ocClient.buildTrait(
		t.Context(),
		"default",
		"project",
		"travel-agent",
		&gen.Component{Metadata: gen.ObjectMeta{Name: "travel-agent"}},
		TraitRequest{
			TraitKind: TraitKindTrait,
			TraitType: TraitAPIManagement,
			Opts: []TraitOption{
				WithAPIPolicies([]APIPolicy{APIKeyAuthPolicy("X-API-Key", "header")}),
			},
		},
	)
	if err != nil {
		t.Fatalf("buildTrait returned error: %v", err)
	}
	if trait.Parameters == nil {
		t.Fatalf("expected trait parameters")
	}

	policies, ok := (*trait.Parameters)["policies"].([]APIPolicy)
	if !ok {
		t.Fatalf("expected []APIPolicy policies, got %#v", (*trait.Parameters)["policies"])
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
	if policies[0].Name != "api-key-auth" {
		t.Fatalf("expected api-key-auth policy, got %q", policies[0].Name)
	}
	if policies[0].Version != "v1" {
		t.Fatalf("expected v1 policy, got %q", policies[0].Version)
	}
	if policies[0].Params["key"] != "X-API-Key" {
		t.Fatalf("expected params.key X-API-Key, got %#v", policies[0].Params["key"])
	}
	if policies[0].Params["in"] != "header" {
		t.Fatalf("expected params.in header, got %#v", policies[0].Params["in"])
	}
}

func TestWithAPIPoliciesNormalizesNilToEmptyArray(t *testing.T) {
	params := map[string]interface{}{}
	WithAPIPolicies(nil)(params)

	policies, ok := params["policies"].([]APIPolicy)
	if !ok {
		t.Fatalf("expected []APIPolicy policies, got %#v", params["policies"])
	}
	if policies == nil {
		t.Fatalf("expected non-nil empty policies slice")
	}
	if len(policies) != 0 {
		t.Fatalf("expected 0 policies, got %d", len(policies))
	}
}

func TestAPIManagementTraitTemplateContainsRestAPIArtifactAnnotation(t *testing.T) {
	content, err := os.ReadFile("../../../../deployments/helm-charts/wso2-amp-platform-resources-extension/templates/component-traits/api-management-trait.yaml")
	if err != nil {
		t.Fatalf("failed to read api-management trait template: %v", err)
	}
	template := string(content)
	if !strings.Contains(template, "artifactID:") {
		t.Fatalf("expected api-management trait schema to include artifactID")
	}
	if !strings.Contains(template, "gateway.api-platform.wso2.com/artifact-id: ${parameters.artifactID}") {
		t.Fatalf("expected RestApi template to include artifact-id annotation")
	}
	if !strings.Contains(template, "x-kubernetes-preserve-unknown-fields: true") {
		t.Fatalf("expected api-management trait policies.items schema to preserve policy object fields")
	}
}
