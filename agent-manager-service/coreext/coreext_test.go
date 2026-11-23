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

package coreext

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

type countingHost struct{ begun int }

type op struct{ id int }

func (h *countingHost) Capability(string) (any, bool) { return nil, false }

func (h *countingHost) Begin(context.Context, string) Operation {
	h.begun++
	return &op{id: h.begun}
}

func (*op) Check(map[string]json.RawMessage) error { return nil }
func (*op) Run(context.Context, *gorm.DB, modkit.Phase, modkit.HookRequest, map[string]json.RawMessage) error {
	return nil
}
func (*op) Emit(context.Context, *gorm.DB, modkit.Event) error { return nil }

// A change made in steps sees one operation, and so one module set, in every
// step; another resource still gets its own.
func TestStartCarriesOneOperation(t *testing.T) {
	h := &countingHost{}
	Set(h)
	defer Set(nil)
	ctx := Start(context.Background(), modkit.ResourceAgent)
	first, second := Begin(ctx, modkit.ResourceAgent), Begin(ctx, modkit.ResourceAgent)
	if first != second || h.begun != 1 {
		t.Fatalf("Begin after Start began %d operations", h.begun)
	}
	if Begin(ctx, modkit.ResourceEnvironment) == first {
		t.Fatal("an operation on another resource reused the agent's")
	}
}

type llmProxy interface{ Proxy() string }

type proxyImpl struct{}

func (proxyImpl) Proxy() string { return "p" }

// Without a host every call is a no-op, but an extension block still has nowhere to go and is rejected.
func TestNoHostIsANoOp(t *testing.T) {
	Set(nil)
	if _, ok := Capability("llm.proxy/v1"); ok {
		t.Fatal("a capability was found with no host installed")
	}
	op := Begin(context.Background(), modkit.ResourceAgent)
	if err := op.Check(nil); err != nil {
		t.Fatalf("Check with no blocks: %v", err)
	}
	if err := op.Run(context.Background(), nil, modkit.Validate, modkit.HookRequest{}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := op.Emit(context.Background(), nil, modkit.Event{}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	err := op.Check(map[string]json.RawMessage{"guardrails": json.RawMessage(`{}`)})
	var mErr *modkit.Error
	if !errors.As(err, &mErr) || mErr.Status != http.StatusBadRequest {
		t.Fatalf("Check with a block: err = %v, want a 400 modkit.Error", err)
	}
	if !strings.Contains(mErr.Message, `extensions.guardrails: no module named "guardrails" is active`) {
		t.Fatalf("message = %q", mErr.Message)
	}
}

func TestStaticCapabilitiesAndLookup(t *testing.T) {
	Set(Static(map[string]any{"llm.proxy/v1": proxyImpl{}, "other/v1": 42}))
	defer Set(nil)

	p, ok := Lookup[llmProxy]("llm.proxy/v1")
	if !ok || p.Proxy() != "p" {
		t.Fatalf("Lookup = %v, %v", p, ok)
	}
	if _, ok := Lookup[llmProxy]("other/v1"); ok {
		t.Fatal("a value that does not implement the interface was returned")
	}
	if _, ok := Lookup[llmProxy]("missing/v1"); ok {
		t.Fatal("a missing capability was found")
	}
	if err := Begin(context.Background(), modkit.ResourceAgent).Check(nil); err != nil {
		t.Fatalf("static host Check: %v", err)
	}
}

func TestBlocks(t *testing.T) {
	got, err := Blocks(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Blocks(nil) = %v, %v", got, err)
	}
	ext := map[string]map[string]any{"guardrails": {"enabled": true}}
	got, err = Blocks(&ext)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["guardrails"]) != `{"enabled":true}` {
		t.Fatalf("block = %s", got["guardrails"])
	}
	bad := map[string]map[string]any{"bad": {"f": func() {}}}
	_, err = Blocks(&bad)
	var mErr *modkit.Error
	if !errors.As(err, &mErr) || !strings.HasPrefix(mErr.Message, "extensions.bad:") {
		t.Fatalf("unencodable block: err = %v, want a rejection naming the block", err)
	}
}
