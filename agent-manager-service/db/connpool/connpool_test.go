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

package connpool

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A statement skipped because its context ended reports that, rather than a nil result and no error.
func TestCancelledContextIsAnError(t *testing.T) {
	sqlDB, err := sql.Open(sqlite.DriverName, "file:"+filepath.Join(t.TempDir(), "c.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	pool := New(sqlDB, RetryParams{MaxRetries: 1}).(*connPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = pool.ExecContext(ctx, "SELECT 1")
	assert.ErrorIs(t, err, context.Canceled)
	_, err = pool.QueryContext(ctx, "SELECT 1")
	assert.ErrorIs(t, err, context.Canceled)
	row := pool.QueryRowContext(ctx, "SELECT 1")
	require.NotNil(t, row)
	assert.ErrorIs(t, row.Err(), context.Canceled)
	_, err = pool.BeginTx(ctx, nil)
	assert.ErrorIs(t, err, context.Canceled)
}
