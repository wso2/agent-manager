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
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen-contract <output-dir>")
		os.Exit(2)
	}
	if err := generate(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "gen-contract:", err)
		os.Exit(1)
	}
}

func generate(outDir string) error {
	if err := os.MkdirAll(filepath.Join(outDir, "kinds"), 0o755); err != nil {
		return err
	}
	for _, k := range Contract {
		schema := renderKindSchema(k)
		path := filepath.Join(outDir, "kinds", k.Kind+".schema.json")
		if err := writeJSON(path, schema); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(outDir, "resource.schema.json"), renderResourceSchema()); err != nil {
		return err
	}
	return writeJSON(filepath.Join(outDir, "span.schema.json"), renderEnvelopeSchema())
}

func renderKindSchema(k KindSpec) map[string]any {
	properties := map[string]any{}
	var required []string
	for _, a := range k.Attributes {
		var prop map[string]any
		if a.Const != "" {
			prop = map[string]any{"const": a.Const}
		} else {
			prop = map[string]any{"type": a.Type}
		}
		if a.MinLen > 0 {
			prop["minLength"] = a.MinLen
		}
		if a.Min != nil {
			prop["minimum"] = *a.Min
		}
		properties[a.Key] = prop
		if a.Required {
			required = append(required, a.Key)
		}
	}

	attributes := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true,
	}
	if required != nil {
		attributes["required"] = required
	} else {
		attributes["required"] = []string{}
	}

	switch k.Kind {
	case "tool":
		attributes["anyOf"] = anyOfRequired(ToolNameAnyOf)
	case "retriever":
		attributes["anyOf"] = anyOfRequired(RetrieverVectorDBAnyOf)
	}

	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     fmt.Sprintf("traceloop/v1/kinds/%s.schema.json", k.Kind),
		"title":   fmt.Sprintf("Traceloop v1 — %s span", k.Kind),
		"type":    "object",
		"properties": map[string]any{
			"name":       map[string]any{"type": "string", "minLength": 1},
			"kind":       map[string]any{"type": "string"},
			"attributes": attributes,
		},
		"required": []string{"name", "kind", "attributes"},
	}
}

// anyOfRequired turns ["a","b","c"] into [{"required":["a"]},{"required":["b"]},…].
// The validator passes if at least one of the listed keys is present.
func anyOfRequired(keys []string) []map[string]any {
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{"required": []string{k}})
	}
	return out
}

func renderResourceSchema() map[string]any {
	return map[string]any{
		"$schema":  "https://json-schema.org/draft/2020-12/schema",
		"$id":      "traceloop/v1/resource.schema.json",
		"title":    "Traceloop v1 — span resource attributes",
		"type":     "object",
		"required": []string{"service.name"},
		"properties": map[string]any{
			"service.name": map[string]any{"type": "string", "minLength": 1},
		},
		"additionalProperties": true,
	}
}

func renderEnvelopeSchema() map[string]any {
	return map[string]any{
		"$schema":  "https://json-schema.org/draft/2020-12/schema",
		"$id":      "traceloop/v1/span.schema.json",
		"title":    "Traceloop v1 — span envelope",
		"type":     "object",
		"required": []string{"name", "kind", "attributes", "traceId", "spanId"},
		"properties": map[string]any{
			"name":         map[string]any{"type": "string", "minLength": 1},
			"kind":         map[string]any{"type": "string"},
			"traceId":      map[string]any{"type": "string", "minLength": 1},
			"spanId":       map[string]any{"type": "string", "minLength": 1},
			"parentSpanId": map[string]any{"type": []string{"string", "null"}},
			"attributes":   map[string]any{"type": "object"},
			"resource":     map[string]any{"type": "object"},
		},
		"additionalProperties": true,
	}
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
