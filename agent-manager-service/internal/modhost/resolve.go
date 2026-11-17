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

package modhost

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// Module states. "available" means compiled in but never installed (no row in modules).
const (
	StateAvailable   = "available"
	StateActive      = "active"
	StateDisabled    = "disabled"
	StateUninstalled = "uninstalled"
)

// Step is one action of an install plan.
type Step struct {
	Module string `json:"module"`
	Action string `json:"action"` // install | enable
}

// ResolveError explains why a lifecycle change cannot happen.
type ResolveError struct {
	Msg  string `json:"error"`
	Hint string `json:"hint,omitempty"`
}

func (e *ResolveError) Error() string {
	if e.Hint != "" {
		return e.Msg + " (" + e.Hint + ")"
	}
	return e.Msg
}

// IsResolveError reports whether err is a user-facing lifecycle error.
func IsResolveError(err error) bool {
	var re *ResolveError
	return errors.As(err, &re)
}

func providersOf(capability string, mods map[string]modkit.Manifest) []string {
	var out []string
	for name, m := range mods {
		if slices.Contains(m.Provides, capability) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func stateOf(name string, states map[string]string) string {
	if s, ok := states[name]; ok {
		return s
	}
	return StateAvailable
}

// Plan works out what has to happen for target to become active: target
// itself plus, with withDeps, the modules that provide its missing required
// capabilities, dependencies first. It never changes anything.
func Plan(target string, mods map[string]modkit.Manifest, states map[string]string, withDeps bool) ([]Step, error) {
	var steps []Step
	planned := map[string]bool{}
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		m, ok := mods[name]
		if !ok {
			return &ResolveError{
				Msg:  fmt.Sprintf("module %q is not compiled into this distribution", name),
				Hint: "modules are linked at build time; use a binary that includes it",
			}
		}
		if m.CoreAPI != modkit.CoreAPIVersion {
			return &ResolveError{Msg: fmt.Sprintf("module %s %s targets core API v%d, this core implements v%d",
				name, m.Version, m.CoreAPI, modkit.CoreAPIVersion)}
		}
		if slices.Contains(path, name) {
			return &ResolveError{Msg: "dependency cycle: " + strings.Join(append(path, name), " -> ")}
		}
		if states[name] == StateActive || planned[name] {
			return nil
		}
		for _, capability := range m.Requires {
			if activeProvider(capability, mods, states, planned) != "" {
				continue
			}
			candidates := providersOf(capability, mods)
			if len(candidates) == 0 {
				return &ResolveError{Msg: fmt.Sprintf("%s requires %s, and no module in this distribution provides it", name, capability)}
			}
			if !withDeps {
				return &ResolveError{
					Msg:  fmt.Sprintf("%s requires %s, provided by %s (%s)", name, capability, candidates[0], stateOf(candidates[0], states)),
					Hint: "install or enable it first, or retry with --with-deps",
				}
			}
			if len(candidates) > 1 {
				return &ResolveError{
					Msg:  fmt.Sprintf("%s requires %s; several modules provide it (%s)", name, capability, strings.Join(candidates, ", ")),
					Hint: "install the one you want first",
				}
			}
			if err := visit(candidates[0], append(path, name)); err != nil {
				return err
			}
		}
		action := "install"
		if states[name] == StateDisabled {
			action = "enable"
		}
		steps = append(steps, Step{Module: name, Action: action})
		planned[name] = true
		return nil
	}
	if err := visit(target, nil); err != nil {
		return nil, err
	}
	return steps, nil
}

func activeProvider(capability string, mods map[string]modkit.Manifest, states map[string]string, planned map[string]bool) string {
	for _, p := range providersOf(capability, mods) {
		if states[p] == StateActive || planned[p] {
			return p
		}
	}
	return ""
}

// Dependents lists the modules that would lose a required capability if
// target stopped providing it. With installed=true it includes disabled
// modules too (used for uninstall: their data still references the provider).
func Dependents(target string, mods map[string]modkit.Manifest, states map[string]string, installed bool) []string {
	var out []string
	m, ok := mods[target]
	if !ok {
		return nil
	}
	for name, other := range mods {
		if name == target {
			continue
		}
		st := states[name]
		if st != StateActive && (!installed || st != StateDisabled) {
			continue
		}
		for _, capability := range other.Requires {
			if !slices.Contains(m.Provides, capability) {
				continue
			}
			// Still satisfied if another active module provides it.
			alt := false
			for _, p := range providersOf(capability, mods) {
				if p != target && states[p] == StateActive {
					alt = true
				}
			}
			if !alt {
				out = append(out, name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// ActivationOrder sorts active modules so providers activate before consumers.
func ActivationOrder(active []string, mods map[string]modkit.Manifest) []string {
	set := map[string]bool{}
	for _, a := range active {
		set[a] = true
	}
	var out []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		for _, c := range mods[n].Requires {
			for _, p := range providersOf(c, mods) {
				if set[p] {
					visit(p)
				}
			}
		}
		out = append(out, n)
	}
	sorted := append([]string(nil), active...)
	sort.Strings(sorted)
	for _, n := range sorted {
		visit(n)
	}
	return out
}
