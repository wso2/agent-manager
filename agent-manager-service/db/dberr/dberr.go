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

// Package dberr classifies database errors the same way on every supported database. It inspects the driver's
// error without replacing it, so callers can still read driver detail when they need it.
package dberr

import (
	"errors"

	gosqlite "github.com/glebarez/go-sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsDuplicateKey reports a unique or primary key violation.
func IsDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	if code, ok := postgresCode(err); ok {
		return code == "23505"
	}
	if code, ok := sqliteCode(err); ok {
		return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return false
}

// IsForeignKeyViolation reports a write or delete that breaks a foreign key.
func IsForeignKeyViolation(err error) bool {
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return true
	}
	if code, ok := postgresCode(err); ok {
		return code == "23503"
	}
	if code, ok := sqliteCode(err); ok {
		return code == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
	}
	return false
}

func postgresCode(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, true
	}
	return "", false
}

func sqliteCode(err error) (int, bool) {
	var sqliteErr *gosqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code(), true
	}
	return 0, false
}
