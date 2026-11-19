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
	"context"
	"fmt"
	"slices"
)

// Desired is one entry of a declarative modules list (amp.yaml `modules:`).
type Desired struct {
	Name string
	// Enabled is false for a module that should be installed but disabled.
	Enabled bool
}

// Reconcile changes the modules to match a declarative list, as the server
// does at boot: every listed module is installed (with the providers it
// needs) and enabled or disabled as listed, and every other installed module
// is disabled unless a module that stays active requires it. Built-in modules
// are always active; listing one disabled is an error. A module listed
// disabled that is already installed is left alone; one that is not installed
// yet is installed and then disabled. Replicas booting together may run it at
// once: a change another replica already made counts as done. It returns the
// steps it took, disables as Action "disable".
func (h *Host) Reconcile(ctx context.Context, want []Desired) ([]Step, error) {
	listed := map[string]bool{}
	for _, d := range want {
		m, ok := h.manifests[d.Name]
		if !ok {
			return nil, &ResolveError{Msg: fmt.Sprintf("module %s is listed but not in this binary (compiled in: %v)", d.Name, h.names())}
		}
		if m.Builtin && !d.Enabled {
			return nil, &ResolveError{Msg: d.Name + " is built in: it is always active and cannot be listed disabled"}
		}
		listed[d.Name] = d.Enabled
	}

	var steps []Step
	for _, d := range want {
		if h.manifests[d.Name].Builtin {
			continue
		}
		st := h.stateNow(d.Name)
		// Enabled: anything but active needs an install (which re-enables a
		// disabled module). Disabled: only a module never installed does.
		needed := (d.Enabled && st != StateActive) || (!d.Enabled && (st == StateAvailable || st == StateUninstalled))
		if !needed {
			continue
		}
		s, err := h.Install(ctx, d.Name, true)
		if err != nil && !h.settled(ctx, d.Name, func(st string) bool { return st == StateActive || (st == StateDisabled && !d.Enabled) }) {
			return steps, fmt.Errorf("installing %s: %w", d.Name, err)
		}
		steps = append(steps, s...)
	}

	// Disable what must not run: listed disabled, or not listed at all.
	var off []string
	for _, name := range h.names() {
		enabled, isListed := listed[name]
		if h.manifests[name].Builtin || (isListed && enabled) || h.stateNow(name) != StateActive {
			continue
		}
		off = append(off, name)
	}
	// Consumers go before their providers: keep disabling whatever no active
	// module requires until nothing more can go.
	for progress := true; progress && len(off) > 0; {
		progress = false
		for _, name := range slices.Clone(off) {
			if len(h.dependents(name)) > 0 {
				continue
			}
			if err := h.Disable(ctx, name); err != nil && !h.settled(ctx, name, func(st string) bool { return st != StateActive }) {
				return steps, fmt.Errorf("disabling %s: %w", name, err)
			} else if err == nil {
				steps = append(steps, Step{Module: name, Action: "disable"})
			}
			off = slices.DeleteFunc(off, func(n string) bool { return n == name })
			progress = true
		}
	}
	for _, name := range off {
		deps := h.dependents(name)
		if _, isListed := listed[name]; isListed {
			return steps, &ResolveError{Msg: fmt.Sprintf("%s is listed disabled, but %v require it", name, deps)}
		}
		h.log.Info("module not listed but kept active: listed modules require it", "module", name, "requiredBy", deps)
	}
	return steps, nil
}

// settled reports, after a lifecycle change failed, whether the module has
// reached the wanted state anyway (another replica made the same change).
func (h *Host) settled(ctx context.Context, name string, ok func(state string) bool) bool {
	if err := h.Reload(ctx); err != nil {
		return false
	}
	if ok(h.stateNow(name)) {
		h.log.Info("module already in the listed state", "module", name, "state", h.stateNow(name))
		return true
	}
	return false
}

func (h *Host) dependents(name string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return Dependents(name, h.manifests, h.states, false)
}

func (h *Host) stateNow(name string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return stateOf(name, h.states)
}
