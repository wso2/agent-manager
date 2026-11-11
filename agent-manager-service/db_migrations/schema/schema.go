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

// Package schema holds the AMP Lite database schema once, in logical types, and renders the baseline DDL for each
// supported database from it.
package schema

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed lite.yaml
var liteYAML []byte

// Logical column types. Each database maps them to its own types (see the schema design's type table).
const (
	TypeID       = "id"
	TypeText     = "text"
	TypeLongText = "longtext"
	TypeJSON     = "json"
	TypeBool     = "bool"
	TypeInt      = "int"
	TypeBigInt   = "bigint"
	TypeTime     = "time"
	TypeBinary   = "binary"
)

// maxNameLength is the longest table, column, index or constraint name allowed on every database.
const maxNameLength = 63

type Schema struct {
	Tables []Table `yaml:"tables"`
}

type Table struct {
	Name        string       `yaml:"name"`
	Columns     []Column     `yaml:"columns"`
	PrimaryKey  []string     `yaml:"primary_key"`
	Unique      [][]string   `yaml:"unique"`
	ForeignKeys []ForeignKey `yaml:"foreign_keys"`
	Checks      []string     `yaml:"checks"`
	Indexes     [][]string   `yaml:"indexes"`
	Seed        []Row        `yaml:"seed"`
}

type Column struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Size     int    `yaml:"size"`
	Nullable bool   `yaml:"nullable"`
}

type ForeignKey struct {
	Columns    []string `yaml:"columns"`
	References string   `yaml:"references"`
	RefColumns []string `yaml:"ref_columns"`
	// OnDelete is "cascade" or empty (no action). Rule 7 allows one cascade path per table.
	OnDelete string `yaml:"on_delete"`
}

// Row is a seed row: column name to literal text value.
type Row map[string]string

// Lite loads the AMP Lite schema.
func Lite() (*Schema, error) {
	return Parse(liteYAML)
}

// Parse reads and validates a schema.
func Parse(data []byte) (*Schema, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing schema: %w", err)
	}
	if err := rejectNullKeys(&root); err != nil {
		return nil, err
	}
	var s Schema
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parsing schema: %w", err)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// rejectNullKeys catches keys YAML reads as null rather than as a name (`null: true` is a null key), which the strict
// decoder otherwise skips without complaint.
func rejectNullKeys(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if k := n.Content[i]; k.Tag == "!!null" {
				return fmt.Errorf("parsing schema: line %d: key %q is read as null; did you mean nullable?", k.Line, k.Value)
			}
		}
	}
	for _, c := range n.Content {
		if err := rejectNullKeys(c); err != nil {
			return err
		}
	}
	return nil
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (s *Schema) table(name string) *Table {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

func (t *Table) column(name string) *Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// keyed reports whether cols are exactly the primary key or one of the unique keys, as a foreign key target needs.
func (t *Table) keyed(cols []string) bool {
	same := func(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }
	if same(t.PrimaryKey, cols) {
		return true
	}
	for _, u := range t.Unique {
		if same(u, cols) {
			return true
		}
	}
	return false
}

func (s *Schema) validate() error {
	var errs []string
	fail := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	seen := map[string]bool{}
	for ti := range s.Tables {
		t := &s.Tables[ti]
		if !namePattern.MatchString(t.Name) || len(t.Name) > maxNameLength {
			fail("table %q: invalid name", t.Name)
		}
		if seen[t.Name] {
			fail("table %q: defined twice", t.Name)
		}
		seen[t.Name] = true
		cols := map[string]bool{}
		for _, c := range t.Columns {
			if !namePattern.MatchString(c.Name) || len(c.Name) > maxNameLength {
				fail("%s.%s: invalid name", t.Name, c.Name)
			}
			if cols[c.Name] {
				fail("%s.%s: defined twice", t.Name, c.Name)
			}
			cols[c.Name] = true
			switch c.Type {
			case TypeText:
				if c.Size <= 0 || c.Size > 4000 {
					fail("%s.%s: text needs a size from 1 to 4000", t.Name, c.Name)
				}
			case TypeID, TypeLongText, TypeJSON, TypeBool, TypeInt, TypeBigInt, TypeTime, TypeBinary:
				if c.Size != 0 {
					fail("%s.%s: only text takes a size", t.Name, c.Name)
				}
			default:
				fail("%s.%s: unknown type %q", t.Name, c.Name, c.Type)
			}
		}
		checkCols := func(what string, list []string) {
			if len(list) == 0 {
				fail("%s: empty %s", t.Name, what)
			}
			for _, c := range list {
				col := t.column(c)
				if col == nil {
					fail("%s: %s names unknown column %q", t.Name, what, c)
					continue
				}
				if col.Type == TypeLongText || col.Type == TypeJSON || col.Type == TypeBinary {
					fail("%s: %s uses %s column %q (rule 5)", t.Name, what, col.Type, c)
				}
			}
		}
		if len(t.PrimaryKey) == 0 {
			fail("%s: no primary key", t.Name)
		}
		checkCols("primary key", t.PrimaryKey)
		for _, c := range t.PrimaryKey {
			if col := t.column(c); col != nil && col.Nullable {
				fail("%s: primary key column %q is nullable", t.Name, c)
			}
		}
		for _, u := range t.Unique {
			checkCols("unique key", u)
			for _, c := range u {
				if col := t.column(c); col != nil && col.Nullable {
					fail("%s: unique key column %q is nullable (rule 8)", t.Name, c)
				}
			}
		}
		for _, ix := range t.Indexes {
			checkCols("index", ix)
		}
		cascades := 0
		for _, fk := range t.ForeignKeys {
			checkCols("foreign key", fk.Columns)
			switch fk.OnDelete {
			case "":
			case "cascade":
				cascades++
			default:
				fail("%s: on_delete %q (use cascade or leave empty)", t.Name, fk.OnDelete)
			}
			ref := s.table(fk.References)
			if ref == nil {
				fail("%s: foreign key references unknown table %q", t.Name, fk.References)
				continue
			}
			if len(fk.RefColumns) != len(fk.Columns) {
				fail("%s: foreign key to %s has %d columns but %d referenced", t.Name, fk.References, len(fk.Columns), len(fk.RefColumns))
				continue
			}
			if !ref.keyed(fk.RefColumns) {
				fail("%s: foreign key to %s(%s) needs a primary or unique key there", t.Name, fk.References, strings.Join(fk.RefColumns, ", "))
			}
			for i, c := range fk.Columns {
				col, rc := t.column(c), ref.column(fk.RefColumns[i])
				if col != nil && rc != nil && (col.Type != rc.Type || col.Size != rc.Size) {
					fail("%s.%s: type differs from %s.%s", t.Name, c, ref.Name, rc.Name)
				}
			}
		}
		if cascades > 1 {
			fail("%s: %d cascading foreign keys (rule 7 allows one)", t.Name, cascades)
		}
		for _, row := range t.Seed {
			for c := range row {
				if t.column(c) == nil {
					fail("%s: seed names unknown column %q", t.Name, c)
				}
			}
		}
	}
	if _, err := s.ordered(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid schema:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// ordered returns the tables with every table after the tables it references, keeping file order otherwise.
func (s *Schema) ordered() ([]*Table, error) {
	done := map[string]bool{}
	var out []*Table
	for len(out) < len(s.Tables) {
		progressed := false
		for i := range s.Tables {
			t := &s.Tables[i]
			if done[t.Name] {
				continue
			}
			ready := true
			for _, fk := range t.ForeignKeys {
				if fk.References != t.Name && !done[fk.References] {
					ready = false
					break
				}
			}
			if ready {
				done[t.Name] = true
				out = append(out, t)
				progressed = true
			}
		}
		if !progressed {
			var left []string
			for _, t := range s.Tables {
				if !done[t.Name] {
					left = append(left, t.Name)
				}
			}
			sort.Strings(left)
			return nil, fmt.Errorf("foreign key cycle among: %s", strings.Join(left, ", "))
		}
	}
	return out, nil
}

// constraintName builds a name within maxNameLength, ending a long one in a short hash so names stay unique.
func constraintName(prefix, table string, cols []string) string {
	name := prefix + "_" + table
	if len(cols) > 0 {
		name += "_" + strings.Join(cols, "_")
	}
	if len(name) <= maxNameLength {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := "_" + hex.EncodeToString(sum[:])[:8]
	return name[:maxNameLength-len(suffix)] + suffix
}
