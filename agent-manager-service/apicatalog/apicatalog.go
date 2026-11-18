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

// Package apicatalog cuts the OpenAPI spec down to the API a running server
// actually serves: the core's operations, those of the active modules, and
// none that answer 501.
//
// Every operation in the spec names its module with x-amp-module (core,
// gateway, legacy, ...); an operation this version answers with 501 also says
// x-amp-not-served: true. api/openapi_catalog_invariant_test.go holds both to
// the route table.
package apicatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	// CoreModule is the x-amp-module of the core's own operations, which
	// every distribution serves.
	CoreModule = "core"
	// ModuleKey names the module that serves an operation.
	ModuleKey = "x-amp-module"
	// NotServedKey marks an operation this version answers with 501.
	NotServedKey = "x-amp-not-served"
	// ModulesKey lists, under info, the modules a filtered spec covers.
	ModulesKey = "x-amp-modules"
)

var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

// Spec is a filtered spec in both encodings.
type Spec struct {
	YAML []byte
	JSON []byte
	// Skipped says why a module's fragment was left out (see Filter).
	Skipped []error
}

// Catalog serves the spec filtered to a set of active modules, rendering each
// set once.
type Catalog struct {
	// OnSkip, if set, is told once per rendered module set about each
	// module fragment left out.
	OnSkip func(error)

	raw   []byte
	mu    sync.Mutex
	cache map[string]*Spec
}

// New returns a catalog of the spec in raw.
func New(raw []byte) *Catalog {
	return &Catalog{raw: raw, cache: map[string]*Spec{}}
}

// For returns the spec of the core and the active modules, with the OpenAPI
// fragments those modules describe their own routes in (by module name; see
// Filter). Pass both from one snapshot of the module host. A spec is cached
// under the module set and the content of the fragments merged into it, so
// the cache never serves a spec built from a different fragment.
func (c *Catalog) For(active []string, fragments map[string][]byte) (*Spec, error) {
	mods := Modules(active)
	key := cacheKey(mods, fragments)
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.cache[key]; ok {
		return s, nil
	}
	doc, skipped, err := Filter(c.raw, mods, fragments)
	if err != nil {
		return nil, err
	}
	var y bytes.Buffer
	enc := yaml.NewEncoder(&y)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode spec as YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode spec as YAML: %w", err)
	}
	var j bytes.Buffer
	if err := writeJSON(&j, doc); err != nil {
		return nil, fmt.Errorf("encode spec as JSON: %w", err)
	}
	s := &Spec{YAML: y.Bytes(), JSON: j.Bytes(), Skipped: skipped}
	if c.OnSkip != nil {
		for _, e := range skipped {
			c.OnSkip(e)
		}
	}
	c.cache[key] = s
	return s, nil
}

// cacheKey names a module set and the fragments of its modules.
func cacheKey(mods []string, fragments map[string][]byte) string {
	h := sha256.New()
	for _, m := range mods {
		if frag, ok := fragments[m]; ok {
			// A hash.Hash never returns an error from Write.
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00", m, len(frag))
			_, _ = h.Write(frag)
		}
	}
	return strings.Join(mods, ",") + "@" + hex.EncodeToString(h.Sum(nil))
}

// Modules is the sorted set of the core and the given modules.
func Modules(active []string) []string {
	mods := append([]string{CoreModule}, active...)
	slices.Sort(mods)
	return slices.Compact(mods)
}

// Filter parses the spec and keeps only the operations of the given modules
// that are served. Components that only the removed operations referenced
// are removed with them; components the whole spec never referenced are left
// alone.
//
// fragments are the OpenAPI documents modules describe the routes they mount
// with Runtime.Handle in, by module name. A fragment's paths are relative to
// the module (/tags), and are merged as /m/<module>/tags with x-amp-module set
// to the module; its components are merged as they are. Only the fragments
// of the given modules are merged. A fragment that does not parse, or whose
// paths or components collide with what is already there, is left out whole
// and reported in skipped.
func Filter(raw []byte, modules []string, fragments map[string][]byte) (doc *yaml.Node, skipped []error, err error) {
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(raw, doc); err != nil {
		return nil, nil, fmt.Errorf("parse spec: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("spec is not a YAML mapping")
	}
	root := doc.Content[0]
	keep := map[string]bool{}
	for _, m := range modules {
		keep[m] = true
	}
	for _, m := range modules {
		if frag, ok := fragments[m]; ok && m != CoreModule {
			if err := mergeFragment(root, m, frag); err != nil {
				skipped = append(skipped, fmt.Errorf("module %s: OpenAPI fragment left out: %w", m, err))
			}
		}
	}

	before := reachable(root, nil)
	if paths := mappingValue(root, "paths"); paths != nil {
		filterPaths(paths, keep)
	}
	// Components nothing referenced to begin with stay, so what they
	// reference must stay too.
	after := reachable(root, unreferenced(root, before))
	pruneComponents(root, before, after)

	if info := mappingValue(root, "info"); info != nil {
		list := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, m := range modules {
			list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: m})
		}
		setMappingValue(info, ModulesKey, list)
	}
	return doc, skipped, nil
}

// filterPaths drops the operations not kept, and path items left with none.
func filterPaths(paths *yaml.Node, keep map[string]bool) {
	var content []*yaml.Node
	for i := 0; i+1 < len(paths.Content); i += 2 {
		item := paths.Content[i+1]
		if item.Kind != yaml.MappingNode {
			content = append(content, paths.Content[i], item)
			continue
		}
		var kept []*yaml.Node
		ops := 0
		for j := 0; j+1 < len(item.Content); j += 2 {
			k, v := item.Content[j], item.Content[j+1]
			if httpMethods[k.Value] {
				if !served(v, keep) {
					continue
				}
				ops++
			}
			kept = append(kept, k, v)
		}
		if ops == 0 {
			continue
		}
		item.Content = kept
		content = append(content, paths.Content[i], item)
	}
	paths.Content = content
}

func served(op *yaml.Node, keep map[string]bool) bool {
	if v := mappingValue(op, NotServedKey); v != nil && v.Value == "true" {
		return false
	}
	mod := CoreModule
	if v := mappingValue(op, ModuleKey); v != nil {
		mod = v.Value
	}
	return keep[mod]
}

// componentRef is a local reference: #/components/<kind>/<name>.
type componentRef struct{ kind, name string }

// reachable is every component the spec's non-component parts and the
// extra components reference, directly or through other components.
func reachable(root *yaml.Node, extra []componentRef) map[componentRef]bool {
	components := mappingValue(root, "components")
	seen := map[componentRef]bool{}
	var queue []componentRef
	visit := func(n *yaml.Node) {
		collectRefs(n, func(r componentRef) {
			if !seen[r] {
				seen[r] = true
				queue = append(queue, r)
			}
		})
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "components" {
			visit(root.Content[i+1])
		}
	}
	queue = append(queue, extra...)
	for len(queue) > 0 {
		r := queue[0]
		queue = queue[1:]
		if kind := mappingValue(components, r.kind); kind != nil {
			if def := mappingValue(kind, r.name); def != nil {
				visit(def)
			}
		}
	}
	return seen
}

// unreferenced lists the components not in referenced.
func unreferenced(root *yaml.Node, referenced map[componentRef]bool) []componentRef {
	components := mappingValue(root, "components")
	if components == nil {
		return nil
	}
	var out []componentRef
	for i := 0; i+1 < len(components.Content); i += 2 {
		kind := components.Content[i+1]
		if kind.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(kind.Content); j += 2 {
			r := componentRef{kind: components.Content[i].Value, name: kind.Content[j].Value}
			if !referenced[r] {
				out = append(out, r)
			}
		}
	}
	return out
}

func collectRefs(n *yaml.Node, fn func(componentRef)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "$ref" && n.Content[i+1].Kind == yaml.ScalarNode {
				if r, ok := parseRef(n.Content[i+1].Value); ok {
					fn(r)
				}
			}
		}
	}
	for _, c := range n.Content {
		collectRefs(c, fn)
	}
}

func parseRef(ref string) (componentRef, bool) {
	rest, ok := strings.CutPrefix(ref, "#/components/")
	if !ok {
		return componentRef{}, false
	}
	kind, name, ok := strings.Cut(rest, "/")
	if !ok || strings.Contains(name, "/") {
		return componentRef{}, false
	}
	name = strings.ReplaceAll(strings.ReplaceAll(name, "~1", "/"), "~0", "~")
	return componentRef{kind: kind, name: name}, true
}

// pruneComponents removes the components referenced before filtering and
// not after, and component sections left empty.
func pruneComponents(root *yaml.Node, before, after map[componentRef]bool) {
	components := mappingValue(root, "components")
	if components == nil || components.Kind != yaml.MappingNode {
		return
	}
	var sections []*yaml.Node
	for i := 0; i+1 < len(components.Content); i += 2 {
		kindKey, kind := components.Content[i], components.Content[i+1]
		if kind.Kind == yaml.MappingNode {
			var kept []*yaml.Node
			for j := 0; j+1 < len(kind.Content); j += 2 {
				r := componentRef{kind: kindKey.Value, name: kind.Content[j].Value}
				if before[r] && !after[r] {
					continue
				}
				kept = append(kept, kind.Content[j], kind.Content[j+1])
			}
			if len(kept) == 0 && len(kind.Content) > 0 {
				continue
			}
			kind.Content = kept
		}
		sections = append(sections, kindKey, kind)
	}
	components.Content = sections
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func setMappingValue(n *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1] = v
			return
		}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}
