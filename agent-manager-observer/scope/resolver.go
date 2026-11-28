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

// Package scope turns the names in a request into the UIDs stored with each span.
package scope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// UIDs says whose data a request may see. An empty field means "no filter on this".
type UIDs struct{ Namespace, Project, Component, Environment string }

// ErrUnknownScope means a name in the request has no matching id.
var ErrUnknownScope = errors.New("unknown scope name")

// Resolver turns the names in a request into permanent ids.
type Resolver interface {
	Resolve(ctx context.Context, s observer.ComponentSearchScope) (UIDs, error)
}

// StaticResolver answers from a fixed list. It is for development and tests.
// Each map key is "namespace/name" and each value is the UID.
type StaticResolver struct {
	Projects     map[string]string `json:"projects"`
	Components   map[string]string `json:"components"`
	Environments map[string]string `json:"environments"`
}

func NewStaticResolverFromJSON(raw string) (StaticResolver, error) {
	var r StaticResolver
	if strings.TrimSpace(raw) == "" {
		return r, nil // empty lists: every named scope is refused, which is safe
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return StaticResolver{}, fmt.Errorf("scope list is not valid JSON: %w", err)
	}
	return r, nil
}

func (r StaticResolver) Resolve(_ context.Context, s observer.ComponentSearchScope) (UIDs, error) {
	if strings.TrimSpace(s.Namespace) == "" {
		return UIDs{}, fmt.Errorf("%w: namespace is required", ErrUnknownScope)
	}
	out := UIDs{Namespace: s.Namespace}
	var err error
	if out.Project, err = lookup(r.Projects, s.Namespace, s.Project, "project"); err != nil {
		return UIDs{}, err
	}
	if out.Component, err = lookup(r.Components, s.Namespace, s.Component, "agent"); err != nil {
		return UIDs{}, err
	}
	if out.Environment, err = lookup(r.Environments, s.Namespace, s.Environment, "environment"); err != nil {
		return UIDs{}, err
	}
	return out, nil
}

// lookup returns "" only when the name was not asked for. A name that was asked for
// but is missing from the list is always an error.
func lookup(m map[string]string, ns string, name *string, kind string) (string, error) {
	if name == nil || *name == "" {
		return "", nil
	}
	uid := m[ns+"/"+*name]
	if uid == "" {
		return "", fmt.Errorf("%w: %s %q in namespace %q", ErrUnknownScope, kind, *name, ns)
	}
	return uid, nil
}
