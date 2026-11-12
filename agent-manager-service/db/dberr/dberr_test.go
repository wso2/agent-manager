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

package dberr_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dberr"
	"github.com/wso2/agent-manager/agent-manager-service/db/sqlitedialect"
)

func TestClassifiesSQLiteErrors(t *testing.T) {
	gormDB, err := gorm.Open(sqlitedialect.Open("file:"+filepath.Join(t.TempDir(), "amp.db")+"?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.Exec("CREATE TABLE parents (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE)").Error)
	require.NoError(t, gormDB.Exec("CREATE TABLE children (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parents(id))").Error)
	require.NoError(t, gormDB.Exec("INSERT INTO parents (id, name) VALUES ('p1', 'one')").Error)

	dupPK := gormDB.Exec("INSERT INTO parents (id, name) VALUES ('p1', 'two')").Error
	dupUnique := gormDB.Exec("INSERT INTO parents (id, name) VALUES ('p2', 'one')").Error
	missingParent := gormDB.Exec("INSERT INTO children (id, parent_id) VALUES ('c1', 'nope')").Error
	require.NoError(t, gormDB.Exec("INSERT INTO children (id, parent_id) VALUES ('c2', 'p1')").Error)
	parentInUse := gormDB.Exec("DELETE FROM parents WHERE id = 'p1'").Error

	assert.True(t, dberr.IsDuplicateKey(dupPK))
	assert.True(t, dberr.IsDuplicateKey(fmt.Errorf("wrapped: %w", dupUnique)))
	assert.False(t, dberr.IsForeignKeyViolation(dupUnique))
	assert.True(t, dberr.IsForeignKeyViolation(missingParent))
	assert.True(t, dberr.IsForeignKeyViolation(parentInUse))
	assert.False(t, dberr.IsDuplicateKey(parentInUse))
}

func TestClassifiesPostgresErrors(t *testing.T) {
	assert.True(t, dberr.IsDuplicateKey(fmt.Errorf("create: %w", &pgconn.PgError{Code: "23505"})))
	assert.True(t, dberr.IsForeignKeyViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, dberr.IsDuplicateKey(&pgconn.PgError{Code: "23503"}))
}

func TestClassifiesTranslatedErrors(t *testing.T) {
	assert.True(t, dberr.IsDuplicateKey(gorm.ErrDuplicatedKey))
	assert.True(t, dberr.IsForeignKeyViolation(gorm.ErrForeignKeyViolated))
	assert.False(t, dberr.IsDuplicateKey(nil))
	assert.False(t, dberr.IsForeignKeyViolation(fmt.Errorf("unrelated")))
}
