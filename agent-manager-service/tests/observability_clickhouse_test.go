//go:build integration

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

package tests

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/clickhouse"
)

// observabilityClickHouse points the observability module at the ClickHouse
// at CLICKHOUSE_TEST_URL (see the service's CI workflow), with the migrator
// login when migrator is true, and skips when it is unset.
func observabilityClickHouse(t *testing.T, migrator bool) {
	t.Helper()
	url := os.Getenv("CLICKHOUSE_TEST_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_TEST_URL is not set")
	}
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_URL", url)
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_USER", "amp_reader")
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_PASSWORD", os.Getenv("AMP_CLICKHOUSE_READER_PASSWORD"))
	// Far off, so the store tests' November 2026 spans never expire.
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_TTL_DAYS", "3650")
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_USER", "")
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_PASSWORD", "")
	if migrator {
		t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_USER", "amp_migrator")
		t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_PASSWORD", os.Getenv("AMP_CLICKHOUSE_MIGRATOR_PASSWORD"))
	}
}

func observabilityHost(mod modkit.Module) *modhost.Host {
	return modhost.NewWith(db.DB(context.Background()), config.GetConfig().Database.Driver, slog.New(slog.DiscardHandler),
		[]modkit.Module{mod})
}

// TestClickHouseMigrate runs `amp migrate`'s module step for the observability
// module against a real ClickHouse.
func TestClickHouseMigrate(t *testing.T) {
	observabilityClickHouse(t, true)
	ctx := context.Background()
	h := observabilityHost(observability.New(nil))

	res, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NotNil(t, res[0].External)
	assert.Equal(t, "clickhouse", res[0].External.Store)
	assert.Equal(t, len(files(t)), res[0].External.Total)

	// The second run applies nothing.
	res, err = h.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, &modhost.ExternalResult{Store: "clickhouse", Total: len(files(t))}, res[0].External)

	// A binary whose applied schema file was edited is refused.
	edited := fstest.MapFS{}
	for _, f := range files(t) {
		body, err := fs.ReadFile(clickhouse.Migrations(), f)
		require.NoError(t, err)
		edited[f] = &fstest.MapFile{Data: append(body, []byte("\n-- edited")...)}
	}
	_, err = observabilityHost(withFiles{Module: observability.New(nil), files: edited}).Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module observability clickhouse schema 001_otel_traces changed after it was applied")
}

// TestClickHouseMigrateWithoutTheMigratorLogin is the API server migrating on
// start: it has the reader login only, so the ClickHouse step is skipped.
func TestClickHouseMigrateWithoutTheMigratorLogin(t *testing.T) {
	observabilityClickHouse(t, false)
	res, err := observabilityHost(observability.New(nil)).Migrate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res[0].External)
	assert.Contains(t, res[0].External.Skipped, "MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_USER")
}

func files(t *testing.T) []string {
	t.Helper()
	out, err := fs.Glob(clickhouse.Migrations(), "*.sql")
	require.NoError(t, err)
	return out
}

// withFiles is the observability module with other ClickHouse schema files,
// as another build would have.
type withFiles struct {
	modkit.Module
	files fs.FS
}

func (m withFiles) ExternalSchema(cfg func(string) string, apply bool) (modkit.ExternalSchema, error) {
	s, err := m.Module.(modkit.ExternalMigrator).ExternalSchema(cfg, apply)
	if err != nil {
		return nil, err
	}
	return schemaFiles{ExternalSchema: s, files: m.files}, nil
}

type schemaFiles struct {
	modkit.ExternalSchema
	files fs.FS
}

func (s schemaFiles) Files() fs.FS { return s.files }
