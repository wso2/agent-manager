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

package schema

import (
	"fmt"
	"sort"
	"strings"
)

// Dialects the baseline is rendered for.
const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

// Dialects lists every dialect with a rendered baseline.
var Dialects = []string{DialectSQLite, DialectPostgres}

// StatementSeparator ends every statement in rendered DDL, so a runner can split a file without parsing SQL.
const StatementSeparator = ";\n"

// Header starts every rendered file.
const Header = "-- Code generated from db_migrations/schema/lite.yaml by `make schema`. DO NOT EDIT.\n"

type dialect struct {
	columnType func(c Column) string
	// columnChecks are checks the column type needs on this database, e.g. JSON validity on SQLite.
	columnChecks func(c Column) []string
	boolLiteral  func(v string) string
}

var dialects = map[string]dialect{
	DialectSQLite: {
		columnType: func(c Column) string {
			switch c.Type {
			case TypeInt, TypeBigInt, TypeBool:
				return "INTEGER"
			case TypeTime:
				// Rule 12: the driver parses stored text back into a time only for TIMESTAMP-like declared types.
				return "TIMESTAMP"
			case TypeBinary:
				return "BLOB"
			default:
				return "TEXT"
			}
		},
		columnChecks: func(c Column) []string {
			var check string
			switch c.Type {
			case TypeJSON:
				check = "json_valid(" + c.Name + ")"
			case TypeBool:
				check = c.Name + " IN (0, 1)"
			default:
				return nil
			}
			// json_valid(NULL) is 0 on SQLite, so a nullable column says NULL is fine.
			if c.Nullable {
				check = c.Name + " IS NULL OR " + check
			}
			return []string{check}
		},
		boolLiteral: func(v string) string {
			if v == "true" {
				return "1"
			}
			return "0"
		},
	},
	DialectPostgres: {
		columnType: func(c Column) string {
			switch c.Type {
			case TypeID:
				return "UUID"
			case TypeText:
				return fmt.Sprintf("VARCHAR(%d)", c.Size)
			case TypeLongText:
				return "TEXT"
			case TypeJSON:
				return "JSONB"
			case TypeBool:
				return "BOOLEAN"
			case TypeInt:
				return "INTEGER"
			case TypeBigInt:
				return "BIGINT"
			case TypeTime:
				return "TIMESTAMPTZ"
			case TypeBinary:
				return "BYTEA"
			}
			return ""
		},
		columnChecks: func(Column) []string { return nil },
		boolLiteral:  func(v string) string { return strings.ToUpper(v) },
	},
}

// Render returns the baseline DDL for one dialect.
func (s *Schema) Render(dialectName string) (string, error) {
	d, ok := dialects[dialectName]
	if !ok {
		return "", fmt.Errorf("unknown dialect %q", dialectName)
	}
	tables, err := s.ordered()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(Header)
	for _, t := range tables {
		b.WriteString("\n")
		renderTable(&b, d, t)
		for _, ix := range t.Indexes {
			fmt.Fprintf(&b, "CREATE INDEX %s ON %s (%s)%s",
				constraintName("idx", t.Name, ix), t.Name, strings.Join(ix, ", "), StatementSeparator)
		}
		for _, row := range t.Seed {
			renderSeed(&b, d, t, row)
		}
	}
	return b.String(), nil
}

func renderTable(b *strings.Builder, d dialect, t *Table) {
	var lines []string
	var checks []string
	for _, c := range t.Columns {
		line := "    " + c.Name + " " + d.columnType(c)
		if !c.Nullable {
			line += " NOT NULL"
		}
		lines = append(lines, line)
		checks = append(checks, d.columnChecks(c)...)
	}
	lines = append(lines, fmt.Sprintf("    CONSTRAINT %s PRIMARY KEY (%s)",
		constraintName("pk", t.Name, nil), strings.Join(t.PrimaryKey, ", ")))
	for _, u := range t.Unique {
		lines = append(lines, fmt.Sprintf("    CONSTRAINT %s UNIQUE (%s)",
			constraintName("uq", t.Name, u), strings.Join(u, ", ")))
	}
	for _, fk := range t.ForeignKeys {
		line := fmt.Sprintf("    CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			constraintName("fk", t.Name, fk.Columns), strings.Join(fk.Columns, ", "),
			fk.References, strings.Join(fk.RefColumns, ", "))
		if fk.OnDelete == "cascade" {
			line += " ON DELETE CASCADE"
		}
		lines = append(lines, line)
	}
	checks = append(checks, t.Checks...)
	for i, c := range checks {
		lines = append(lines, fmt.Sprintf("    CONSTRAINT %s CHECK (%s)",
			constraintName("ck", t.Name, []string{fmt.Sprint(i + 1)}), c))
	}
	fmt.Fprintf(b, "CREATE TABLE %s (\n%s\n)%s", t.Name, strings.Join(lines, ",\n"), StatementSeparator)
}

func renderSeed(b *strings.Builder, d dialect, t *Table, row Row) {
	cols := make([]string, 0, len(row))
	for c := range row {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	vals := make([]string, len(cols))
	for i, c := range cols {
		v := row[c]
		if t.column(c).Type == TypeBool {
			vals[i] = d.boolLiteral(v)
		} else {
			vals[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
		}
	}
	fmt.Fprintf(b, "INSERT INTO %s (%s) VALUES (%s)%s", t.Name, strings.Join(cols, ", "), strings.Join(vals, ", "), StatementSeparator)
}

// BaselinePath is where the rendered baseline for a dialect lives, relative to the module root.
func BaselinePath(dialect string) string {
	return "db_migrations/sql/" + dialect + "/001_baseline.sql"
}
