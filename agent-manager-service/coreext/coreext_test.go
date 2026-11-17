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
	"testing"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

type countingHost struct{ begun int }

type op struct{ id int }

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
