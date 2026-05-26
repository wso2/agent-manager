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

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderKindSchemaIncludesRequiredKeys(t *testing.T) {
	k := KindSpec{
		Kind: "llm",
		Attributes: []AttributeSpec{
			{Key: "gen_ai.system", Type: "string", Required: true},
			{Key: "traceloop.span.kind", Type: "string", Required: true, Const: "llm"},
		},
	}
	b, _ := json.Marshal(renderKindSchema(k))
	s := string(b)
	if !strings.Contains(s, `"gen_ai.system"`) {
		t.Fatalf("missing required key in output: %s", s)
	}
	if !strings.Contains(s, `"const":"llm"`) {
		t.Fatalf("missing const for traceloop.span.kind: %s", s)
	}
}

func TestRenderKindSchemaEmptyRequiredWhenNoneMarked(t *testing.T) {
	k := KindSpec{
		Kind: "unknown",
	}
	b, _ := json.Marshal(renderKindSchema(k))
	s := string(b)
	// JSON should include an empty required array, not omit it.
	if !strings.Contains(s, `"required":[]`) {
		t.Fatalf("expected empty required array, got: %s", s)
	}
}

func TestRenderToolSchemaEmitsAnyOfForNameKeys(t *testing.T) {
	k := KindSpec{
		Kind: "tool",
		Attributes: []AttributeSpec{
			{Key: "traceloop.entity.name", Type: "string"},
		},
	}
	b, _ := json.Marshal(renderKindSchema(k))
	s := string(b)
	for _, key := range ToolNameAnyOf {
		if !strings.Contains(s, key) {
			t.Fatalf("anyOf missing %q in tool schema: %s", key, s)
		}
	}
}

func TestContractCoversAllNineKinds(t *testing.T) {
	expected := map[string]bool{
		"llm": false, "embedding": false, "tool": false,
		"retriever": false, "rerank": false, "agent": false,
		"chain": false, "crewaitask": false, "unknown": false,
	}
	for _, k := range Contract {
		if _, ok := expected[k.Kind]; !ok {
			t.Fatalf("unexpected kind in Contract: %q", k.Kind)
		}
		expected[k.Kind] = true
	}
	for kind, seen := range expected {
		if !seen {
			t.Fatalf("Contract missing kind: %q", kind)
		}
	}
}

func TestRenderEnvelopeRequiresIDFields(t *testing.T) {
	b, _ := json.Marshal(renderEnvelopeSchema())
	s := string(b)
	for _, k := range []string{"traceId", "spanId", "name", "kind", "attributes"} {
		if !strings.Contains(s, `"`+k+`"`) {
			t.Fatalf("envelope missing %q: %s", k, s)
		}
	}
}

func TestRenderResourceRequiresServiceName(t *testing.T) {
	b, _ := json.Marshal(renderResourceSchema())
	s := string(b)
	if !strings.Contains(s, `"required":["service.name"]`) {
		t.Fatalf("resource schema missing required service.name: %s", s)
	}
}
