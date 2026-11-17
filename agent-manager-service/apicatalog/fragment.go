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

package apicatalog

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModulePathPrefix is where the module host mounts a module's
// Runtime.Handle routes, relative to the spec's /api/v1 server URL.
func ModulePathPrefix(module string) string { return "/m/" + module }

// mergeFragment adds a module's fragment to root, or changes nothing and
// says why.
func mergeFragment(root *yaml.Node, module string, raw []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("not a YAML mapping")
	}
	frag := doc.Content[0]

	// Check everything before changing anything.
	paths := mappingValue(root, "paths")
	if paths == nil {
		return errors.New("the spec has no paths")
	}
	fragPaths := mappingValue(frag, "paths")
	var newPaths []*yaml.Node
	if fragPaths != nil {
		if fragPaths.Kind != yaml.MappingNode {
			return errors.New("paths is not a mapping")
		}
		for i := 0; i+1 < len(fragPaths.Content); i += 2 {
			p, item := fragPaths.Content[i].Value, fragPaths.Content[i+1]
			if !strings.HasPrefix(p, "/") {
				return fmt.Errorf("path %q does not start with /", p)
			}
			if item.Kind != yaml.MappingNode {
				return fmt.Errorf("path %s is not a mapping", p)
			}
			full := ModulePathPrefix(module) + p
			if mappingValue(paths, full) != nil {
				return fmt.Errorf("path %s is already in the spec", full)
			}
			for j := 0; j+1 < len(item.Content); j += 2 {
				if !httpMethods[item.Content[j].Value] {
					continue
				}
				op := item.Content[j+1]
				if v := mappingValue(op, ModuleKey); v != nil && v.Value != module {
					return fmt.Errorf("%s %s says %s: %s", item.Content[j].Value, p, ModuleKey, v.Value)
				}
			}
			newPaths = append(newPaths, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: full}, item)
		}
	}
	fragComps := mappingValue(frag, "components")
	comps := mappingValue(root, "components")
	if fragComps != nil {
		if fragComps.Kind != yaml.MappingNode {
			return errors.New("components is not a mapping")
		}
		for i := 0; i+1 < len(fragComps.Content); i += 2 {
			kind, defs := fragComps.Content[i].Value, fragComps.Content[i+1]
			if defs.Kind != yaml.MappingNode {
				return fmt.Errorf("components.%s is not a mapping", kind)
			}
			for j := 0; j+1 < len(defs.Content); j += 2 {
				if mappingValue(mappingValue(comps, kind), defs.Content[j].Value) != nil {
					return fmt.Errorf("components.%s.%s is already in the spec; prefix the module's component names",
						kind, defs.Content[j].Value)
				}
			}
		}
	}

	for i := 0; i+1 < len(newPaths); i += 2 {
		item := newPaths[i+1]
		for j := 0; j+1 < len(item.Content); j += 2 {
			if httpMethods[item.Content[j].Value] {
				setMappingValue(item.Content[j+1], ModuleKey, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: module})
			}
		}
	}
	paths.Content = append(paths.Content, newPaths...)
	if fragComps != nil {
		if comps == nil {
			comps = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMappingValue(root, "components", comps)
		}
		for i := 0; i+1 < len(fragComps.Content); i += 2 {
			kind := fragComps.Content[i].Value
			target := mappingValue(comps, kind)
			if target == nil {
				target = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				setMappingValue(comps, kind, target)
			}
			target.Content = append(target.Content, fragComps.Content[i+1].Content...)
		}
	}
	return nil
}
