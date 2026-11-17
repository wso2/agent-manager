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
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"gopkg.in/yaml.v3"
)

// writeJSON writes a YAML tree as JSON, keeping the order of mapping keys so
// the JSON reads like the YAML (info, then paths, then components).
func writeJSON(buf *bytes.Buffer, n *yaml.Node) error {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) != 1 {
			return fmt.Errorf("document with %d roots", len(n.Content))
		}
		return writeJSON(buf, n.Content[0])
	case yaml.AliasNode:
		return writeJSON(buf, n.Alias)
	case yaml.MappingNode:
		buf.WriteByte('{')
		for i := 0; i+1 < len(n.Content); i += 2 {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeString(buf, n.Content[i].Value); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeJSON(buf, n.Content[i+1]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case yaml.SequenceNode:
		buf.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSON(buf, c); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case yaml.ScalarNode:
		return writeScalar(buf, n)
	default:
		return fmt.Errorf("unexpected YAML node kind %d at line %d", n.Kind, n.Line)
	}
}

func writeScalar(buf *bytes.Buffer, n *yaml.Node) error {
	switch n.ShortTag() {
	case "!!null":
		buf.WriteString("null")
		return nil
	case "!!bool", "!!int", "!!float":
		var v any
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
		if f, ok := v.(float64); ok && (math.IsInf(f, 0) || math.IsNaN(f)) {
			return writeString(buf, n.Value)
		}
		out, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
		buf.Write(out)
		return nil
	default:
		// Strings, and timestamps and the like, which OpenAPI means as text.
		return writeString(buf, n.Value)
	}
}

func writeString(buf *bytes.Buffer, s string) error {
	out, err := json.Marshal(s)
	if err != nil {
		return err
	}
	buf.Write(out)
	return nil
}
