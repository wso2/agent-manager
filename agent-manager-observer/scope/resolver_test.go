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

package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

func TestStaticResolver(t *testing.T) {
	r := StaticResolver{
		Projects:   map[string]string{"default/default": "55f1"},
		Components: map[string]string{"default/it-support": "f412"},
	}
	ctx := context.Background()
	str := func(s string) *string { return &s }

	got, err := r.Resolve(ctx, observer.ComponentSearchScope{
		Namespace: "default", Project: str("default"), Component: str("it-support")})
	if err != nil || got.Project != "55f1" || got.Component != "f412" || got.Environment != "" {
		t.Fatalf("good names: got %+v, err %v", got, err)
	}

	// The dangerous case: a typo must be an error, never "no filter".
	_, err = r.Resolve(ctx, observer.ComponentSearchScope{Namespace: "default", Project: str("billng")})
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("typo in project name must fail, got %v", err)
	}

	_, err = r.Resolve(ctx, observer.ComponentSearchScope{Project: str("default")})
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("missing namespace must fail, got %v", err)
	}
}

func TestNewStaticResolverFromJSON(t *testing.T) {
	// Test 1: Valid JSON
	validJSON := `{"projects": {"default/proj1": "uid-1"}}`
	r, err := NewStaticResolverFromJSON(validJSON)
	if err != nil {
		t.Fatalf("expected no error for valid JSON, got %v", err)
	}
	if r.Projects["default/proj1"] != "uid-1" {
		t.Fatalf("expected project uid-1, got %q", r.Projects["default/proj1"])
	}

	// Test 2: Empty String (Should safely return empty resolver)
	r, err = NewStaticResolverFromJSON("   ")
	if err != nil {
		t.Fatalf("expected no error for empty string, got %v", err)
	}
	if len(r.Projects) != 0 {
		t.Fatalf("expected empty resolver, got %d projects", len(r.Projects))
	}

	// Test 3: Malformed JSON
	badJSON := `{broken-json}`
	_, err = NewStaticResolverFromJSON(badJSON)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}