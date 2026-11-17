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

// Package coreext is how the core's resource code reaches the active modules:
// it checks the extension blocks of a request, runs the modules' resource
// hooks inside the core transaction, and records core events for them. The
// module host installs the implementation at startup; until then, and in
// processes that run no modules, every call is a no-op.
package coreext

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// Extensions is implemented by the module host.
type Extensions interface {
	// Check rejects extension blocks addressed to a module that is not
	// active or takes no hooks on resource.
	Check(resource string, ext map[string]json.RawMessage) error
	// Run runs the hooks of one phase inside tx. It returns the first
	// hook's error.
	Run(ctx context.Context, tx *gorm.DB, phase modkit.Phase, req modkit.HookRequest, ext map[string]json.RawMessage) error
	// Emit records a core event for module subscribers inside tx.
	Emit(ctx context.Context, tx *gorm.DB, e modkit.Event) error
}

type holder struct{ Extensions }

var current atomic.Pointer[holder]

// Set installs the module host. nil restores the no-op.
func Set(x Extensions) {
	if x == nil {
		current.Store(nil)
		return
	}
	current.Store(&holder{x})
}

// Get returns the installed implementation, or a no-op.
func Get() Extensions {
	if h := current.Load(); h != nil {
		return h.Extensions
	}
	return none{}
}

type none struct{}

func (none) Check(_ string, ext map[string]json.RawMessage) error {
	for name := range ext {
		return modkit.Reject("extensions.%s: no module named %q is active", name, name)
	}
	return nil
}

func (none) Run(context.Context, *gorm.DB, modkit.Phase, modkit.HookRequest, map[string]json.RawMessage) error {
	return nil
}

func (none) Emit(context.Context, *gorm.DB, modkit.Event) error { return nil }

// Blocks converts a request's extensions object into raw blocks.
func Blocks(ext *map[string]map[string]any) (map[string]json.RawMessage, error) {
	if ext == nil || len(*ext) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	out := make(map[string]json.RawMessage, len(*ext))
	for name, block := range *ext {
		raw, err := json.Marshal(block)
		if err != nil {
			return nil, modkit.Reject("extensions.%s: %v", name, err)
		}
		out[name] = raw
	}
	return out, nil
}
