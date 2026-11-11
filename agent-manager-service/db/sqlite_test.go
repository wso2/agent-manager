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

package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

func openTestSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "amp.db")
	gormDB, err := Open(config.DatabaseConfig{Driver: config.DBDriverSQLite, SQLitePath: path}, config.POSTGRESQL{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := gormDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	assert.FileExists(t, path)
	return gormDB
}

func TestOpenSQLiteAppliesConnectionSettings(t *testing.T) {
	gormDB := openTestSQLite(t)

	var foreignKeys int
	require.NoError(t, gormDB.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error)
	assert.Equal(t, 1, foreignKeys)

	var journalMode string
	require.NoError(t, gormDB.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	assert.Equal(t, "wal", journalMode)

	assert.Equal(t, time.UTC, gormDB.NowFunc().Location())
}

// SQLite stores times as text, so ordering and range filters must agree with time order. The column must be declared
// TIMESTAMP (or DATE/DATETIME): the driver parses text back into a time only for those declared types.
func TestSQLiteTimesSortInTimeOrder(t *testing.T) {
	gormDB := openTestSQLite(t)
	type row struct {
		ID int
		At time.Time
	}
	require.NoError(t, gormDB.Exec("CREATE TABLE rows (id INTEGER PRIMARY KEY, at TIMESTAMP NOT NULL)").Error)

	base := time.Date(2026, 11, 11, 10, 0, 5, 0, time.UTC)
	times := []time.Time{
		base.Add(500 * time.Millisecond),
		base,
		base.Add(123456 * time.Microsecond),
		base.Add(100 * time.Millisecond),
		base.Add(time.Second),
	}
	for i, at := range times {
		require.NoError(t, gormDB.Table("rows").Create(&row{ID: i + 1, At: at}).Error)
	}

	var got []row
	require.NoError(t, gormDB.Table("rows").Order("at").Find(&got).Error)
	var ids []int
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []int{2, 4, 3, 1, 5}, ids)

	var count int64
	require.NoError(t, gormDB.Table("rows").Where("at > ? AND at < ?", base, base.Add(time.Second)).Count(&count).Error)
	assert.EqualValues(t, 3, count)

	// CAST reads the stored text; scanning the TIMESTAMP column into a string would format the parsed time as RFC 3339.
	var stored []string
	require.NoError(t, gormDB.Raw("SELECT CAST(at AS TEXT) FROM rows ORDER BY id").Scan(&stored).Error)
	assert.Equal(t, "2026-11-11 10:00:05.5+00:00", stored[0])
	assert.Equal(t, "2026-11-11 10:00:05+00:00", stored[1])

	var first row
	require.NoError(t, gormDB.Table("rows").Where("id = ?", 3).Take(&first).Error)
	assert.True(t, first.At.Equal(times[2]), "round trip: got %v, want %v", first.At, times[2])
}
