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

// Package sqlitedialect is a GORM dialector for SQLite over modernc.org/sqlite, the pure-Go SQLite driver, so the
// service builds without cgo. The schema is created by SQL migrations, so the migrator only answers the questions
// gormigrate and the tests ask; it does not alter tables.
package sqlitedialect

import (
	"database/sql"
	"errors"
	"maps"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// DriverName is the database/sql driver name modernc.org/sqlite registers.
const DriverName = "sqlite"

// Dialector opens DSN with the modernc.org/sqlite driver, or uses Conn when it is set.
type Dialector struct {
	DSN  string
	Conn gorm.ConnPool
}

// Open returns a dialector for dsn.
func Open(dsn string) gorm.Dialector {
	return &Dialector{DSN: dsn}
}

func (d Dialector) Name() string { return "sqlite" }

func (d Dialector) Initialize(db *gorm.DB) error {
	if d.Conn != nil {
		db.ConnPool = d.Conn
	} else {
		conn, err := sql.Open(DriverName, d.DSN)
		if err != nil {
			return err
		}
		db.ConnPool = conn
	}
	// The bundled SQLite is newer than 3.35, so RETURNING is always available.
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
		CreateClauses:        []string{"INSERT", "VALUES", "ON CONFLICT", "RETURNING"},
		UpdateClauses:        []string{"UPDATE", "SET", "FROM", "WHERE", "RETURNING"},
		DeleteClauses:        []string{"DELETE", "FROM", "WHERE", "RETURNING"},
		LastInsertIDReversed: true,
	})
	maps.Copy(db.ClauseBuilders, clauseBuilders())
	return nil
}

func clauseBuilders() map[string]clause.ClauseBuilder {
	return map[string]clause.ClauseBuilder{
		// SQLite needs a LIMIT before an OFFSET; -1 means no limit.
		"LIMIT": func(c clause.Clause, builder clause.Builder) {
			limit, ok := c.Expression.(clause.Limit)
			if !ok {
				return
			}
			lmt := -1
			if limit.Limit != nil && *limit.Limit >= 0 {
				lmt = *limit.Limit
			}
			if lmt >= 0 || limit.Offset > 0 {
				_, _ = builder.WriteString("LIMIT " + strconv.Itoa(lmt))
			}
			if limit.Offset > 0 {
				_, _ = builder.WriteString(" OFFSET " + strconv.Itoa(limit.Offset))
			}
		},
		// SQLite has no row locks: a write transaction already holds the database lock.
		"FOR": func(c clause.Clause, builder clause.Builder) {
			if _, ok := c.Expression.(clause.Locking); ok {
				return
			}
			c.Build(builder)
		},
	}
}

func (d Dialector) DefaultValueOf(field *schema.Field) clause.Expression {
	if field.AutoIncrement {
		return clause.Expr{SQL: "NULL"}
	}
	return clause.Expr{SQL: "DEFAULT"}
}

func (d Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return Migrator{migrator.Migrator{Config: migrator.Config{
		DB:                          db,
		Dialector:                   d,
		CreateIndexAfterCreateTable: true,
	}}}
}

func (d Dialector) BindVarTo(writer clause.Writer, _ *gorm.Statement, _ any) {
	_ = writer.WriteByte('?')
}

// QuoteTo double-quotes each dot-separated part of an identifier.
func (d Dialector) QuoteTo(writer clause.Writer, str string) {
	parts := strings.Split(str, ".")
	for i, part := range parts {
		parts[i] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
	}
	_, _ = writer.WriteString(strings.Join(parts, "."))
}

func (d Dialector) Explain(sql string, vars ...any) string {
	return logger.ExplainSQL(sql, nil, `'`, vars...)
}

func (d Dialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Bool:
		return "numeric"
	case schema.Int, schema.Uint:
		if field.AutoIncrement {
			return "integer PRIMARY KEY AUTOINCREMENT"
		}
		return "integer"
	case schema.Float:
		return "real"
	case schema.String:
		return "text"
	case schema.Time:
		if typ, ok := field.TagSettings["TYPE"]; ok {
			return typ
		}
		return "datetime"
	case schema.Bytes:
		return "blob"
	}
	return string(field.DataType)
}

func (d Dialector) SavePoint(tx *gorm.DB, name string) error {
	return tx.Exec("SAVEPOINT " + name).Error
}

func (d Dialector) RollbackTo(tx *gorm.DB, name string) error {
	return tx.Exec("ROLLBACK TO SAVEPOINT " + name).Error
}

// Translate maps constraint violations to GORM's errors when gorm.Config.TranslateError is set.
func (d Dialector) Translate(err error) error {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return err
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return gorm.ErrDuplicatedKey
	case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
		return gorm.ErrForeignKeyViolated
	}
	return err
}
