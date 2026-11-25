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
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type widget struct {
	ID     int    `gorm:"primaryKey;autoIncrement"`
	Name   string `gorm:"uniqueIndex:idx_widget_name"`
	Weight float64
}

type part struct {
	ID       int `gorm:"primaryKey"`
	WidgetID int
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "t.db") + "?_pragma=foreign_keys(1)"
	g, err := gorm.Open(Open(dsn), &gorm.Config{Logger: logger.Discard, TranslateError: true})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, g.Exec(`CREATE TABLE widgets (id integer PRIMARY KEY AUTOINCREMENT, name text NOT NULL, weight real)`).Error)
	require.NoError(t, g.Exec(`CREATE UNIQUE INDEX idx_widget_name ON widgets(name)`).Error)
	require.NoError(t, g.Exec(`CREATE TABLE parts (id integer PRIMARY KEY, widget_id integer NOT NULL REFERENCES widgets(id))`).Error)
	return g
}

func TestCreateReturnsAutoIncrementID(t *testing.T) {
	g := openTestDB(t)
	w := widget{Name: "a"}
	require.NoError(t, g.Create(&w).Error)
	assert.Positive(t, w.ID)
}

func TestTranslateMapsConstraintViolations(t *testing.T) {
	g := openTestDB(t)
	w := widget{Name: "dup"}
	require.NoError(t, g.Create(&w).Error)

	err := g.Create(&widget{Name: "dup"}).Error
	require.ErrorIs(t, err, gorm.ErrDuplicatedKey)

	err = g.Create(&widget{ID: w.ID, Name: "other"}).Error
	require.ErrorIs(t, err, gorm.ErrDuplicatedKey, "primary key clash")

	err = g.Create(&part{ID: 1, WidgetID: 999}).Error
	require.ErrorIs(t, err, gorm.ErrForeignKeyViolated)

	// A constraint other than unique/FK and a non-SQLite error pass through unchanged.
	err = g.Exec("INSERT INTO widgets (name) VALUES (NULL)").Error
	require.Error(t, err)
	assert.NotErrorIs(t, err, gorm.ErrDuplicatedKey)
	assert.NotErrorIs(t, err, gorm.ErrForeignKeyViolated)
	plain := errors.New("plain")
	assert.Same(t, plain, Dialector{}.Translate(plain))
}

func TestLimitOffset(t *testing.T) {
	g := openTestDB(t)
	for _, n := range []string{"a", "b", "c", "d"} {
		require.NoError(t, g.Create(&widget{Name: n}).Error)
	}
	names := func(q *gorm.DB) []string {
		var got []string
		require.NoError(t, q.Model(&widget{}).Order("name").Pluck("name", &got).Error)
		return got
	}
	assert.Equal(t, []string{"a", "b"}, names(g.Limit(2)))
	// An offset without a limit must still work: SQLite needs "LIMIT -1 OFFSET n".
	assert.Equal(t, []string{"c", "d"}, names(g.Offset(2)))
	assert.Equal(t, []string{"b", "c"}, names(g.Limit(2).Offset(1)))
	assert.Equal(t, []string{"a", "b", "c", "d"}, names(g.Limit(-1)))

	sqlText := g.ToSQL(func(tx *gorm.DB) *gorm.DB { return tx.Model(&widget{}).Offset(3).Find(&[]widget{}) })
	assert.Contains(t, sqlText, "LIMIT -1 OFFSET 3")
}

func TestRowLockingClauseIsDropped(t *testing.T) {
	g := openTestDB(t)
	require.NoError(t, g.Create(&widget{Name: "locked"}).Error)
	var got []widget
	err := g.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Find(&got).Error
	})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	sqlText := g.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Find(&[]widget{})
	})
	assert.NotContains(t, strings.ToUpper(sqlText), "FOR UPDATE")
}

func TestSavePointRollback(t *testing.T) {
	g := openTestDB(t)
	err := g.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&widget{Name: "kept"}).Error)
		// A nested transaction that fails rolls back to its savepoint only.
		nestedErr := tx.Transaction(func(tx2 *gorm.DB) error {
			require.NoError(t, tx2.Create(&widget{Name: "dropped"}).Error)
			return errors.New("abort nested")
		})
		require.Error(t, nestedErr)
		return nil
	})
	require.NoError(t, err)
	var got []string
	require.NoError(t, g.Model(&widget{}).Pluck("name", &got).Error)
	assert.Equal(t, []string{"kept"}, got)
}

func TestMigratorReadsSQLiteCatalog(t *testing.T) {
	g := openTestDB(t)
	m := g.Migrator()

	assert.True(t, m.HasTable(&widget{}))
	assert.True(t, m.HasTable("parts"))
	assert.False(t, m.HasTable("missing"))

	tables, err := m.GetTables()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"widgets", "parts"}, tables)

	assert.True(t, m.HasColumn(&widget{}, "Weight"), "field name resolves to its column")
	assert.True(t, m.HasColumn(&widget{}, "name"))
	assert.False(t, m.HasColumn(&widget{}, "colour"))

	assert.True(t, m.HasIndex(&widget{}, "idx_widget_name"))
	assert.False(t, m.HasIndex(&widget{}, "idx_missing"))
	require.NoError(t, m.DropIndex(&widget{}, "idx_widget_name"))
	assert.False(t, m.HasIndex(&widget{}, "idx_widget_name"))
	require.NoError(t, m.DropIndex(&widget{}, "idx_widget_name"), "dropping a missing index is not an error")

	assert.Equal(t, "main", m.CurrentDatabase())
}

func TestInitializeUsesProvidedConn(t *testing.T) {
	sqlDB, err := sql.Open(DriverName, "file:"+filepath.Join(t.TempDir(), "c.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	g, err := gorm.Open(&Dialector{Conn: sqlDB}, &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	assert.Equal(t, "sqlite", g.Name())
	var one int
	require.NoError(t, g.Raw("SELECT 1").Scan(&one).Error)
	assert.Equal(t, 1, one)
}

func TestDialectorHelpers(t *testing.T) {
	d := Dialector{}

	var sb strings.Builder
	d.QuoteTo(&sb, `main.we"ird`)
	assert.Equal(t, `"main"."we""ird"`, sb.String())

	sb.Reset()
	d.BindVarTo(&sb, nil, 1)
	assert.Equal(t, "?", sb.String())

	assert.Equal(t, "SELECT 'x'", d.Explain("SELECT ?", "x"))

	assert.Equal(t, clause.Expr{SQL: "NULL"}, d.DefaultValueOf(&schema.Field{AutoIncrement: true}))
	assert.Equal(t, clause.Expr{SQL: "DEFAULT"}, d.DefaultValueOf(&schema.Field{}))

	cases := []struct {
		field *schema.Field
		want  string
	}{
		{&schema.Field{DataType: schema.Bool}, "numeric"},
		{&schema.Field{DataType: schema.Int}, "integer"},
		{&schema.Field{DataType: schema.Uint, AutoIncrement: true}, "integer PRIMARY KEY AUTOINCREMENT"},
		{&schema.Field{DataType: schema.Float}, "real"},
		{&schema.Field{DataType: schema.String}, "text"},
		{&schema.Field{DataType: schema.Time}, "datetime"},
		{&schema.Field{DataType: schema.Time, TagSettings: map[string]string{"TYPE": "timestamp"}}, "timestamp"},
		{&schema.Field{DataType: schema.Bytes}, "blob"},
		{&schema.Field{DataType: "jsonb"}, "jsonb"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, d.DataTypeOf(tc.field), "DataTypeOf(%s)", tc.field.DataType)
	}
}
