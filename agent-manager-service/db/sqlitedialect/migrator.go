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

package sqlitedialect

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
)

// Migrator reads SQLite's catalog where GORM's generic migrator would query information_schema, which SQLite lacks.
// Creating tables and indexes uses the generic migrator.
type Migrator struct {
	migrator.Migrator
}

func (m Migrator) HasTable(value any) bool {
	var count int
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		return m.DB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", stmt.Table).
			Row().Scan(&count)
	})
	return count > 0
}

func (m Migrator) GetTables() ([]string, error) {
	var tables []string
	err := m.DB.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").
		Scan(&tables).Error
	return tables, err
}

func (m Migrator) HasColumn(value any, name string) bool {
	var count int
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if field := stmt.Schema.LookUpField(name); field != nil {
				name = field.DBName
			}
		}
		return m.DB.Raw("SELECT count(*) FROM pragma_table_info(?) WHERE name = ?", stmt.Table, name).
			Row().Scan(&count)
	})
	return count > 0
}

func (m Migrator) HasIndex(value any, name string) bool {
	var count int
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}
		return m.DB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?",
			stmt.Table, name).Row().Scan(&count)
	})
	return count > 0
}

// DropIndex drops by name alone: SQLite index names are unique per database and DROP INDEX takes no table.
func (m Migrator) DropIndex(value any, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}
		return m.DB.Exec("DROP INDEX IF EXISTS ?", clause.Column{Name: name}).Error
	})
}

func (m Migrator) CurrentDatabase() string {
	var (
		seq  int
		name string
		file string
	)
	_ = m.DB.Raw("PRAGMA database_list").Row().Scan(&seq, &name, &file)
	return name
}
