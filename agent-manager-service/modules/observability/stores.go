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

package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/clickhouse"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/memory"
)

// defaultTTLDays is how long ClickHouse keeps spans when ttl-days is unset.
const defaultTTLDays = 30

// errNoReader refuses a ClickHouse URL without the reader login, which
// ClickHouse would otherwise take as its default user.
var errNoReader = errors.New("observability: clickhouse.url is set but clickhouse.user is not (MODULE_OBSERVABILITY_CLICKHOUSE_USER)")

// agentCleaner deletes an agent's spans.
type agentCleaner interface {
	DeleteAgent(ctx context.Context, orgID, agentID string) error
}

// openStores returns the ClickHouse store, read as the reader login, and the
// cleaner login's store when clickhouse.url is set; else one empty in-memory
// store for both. It does not connect.
func openStores(rt modkit.Runtime) (store.TraceStore, agentCleaner, error) {
	url := rt.Config("clickhouse.url")
	if url == "" {
		rt.Logger().Warn("observability: no ClickHouse configured (MODULE_OBSERVABILITY_CLICKHOUSE_URL); serving traces from an empty in-memory store")
		m := memory.New()
		return m, m, nil
	}
	cfg := clickhouse.Config{URL: url, Database: rt.Config("clickhouse.database")}
	reader := cfg
	reader.User, reader.Password = rt.Config("clickhouse.user"), rt.Config("clickhouse.password")
	if reader.User == "" {
		return nil, nil, errNoReader
	}
	traces, err := clickhouse.New(reader)
	if err != nil {
		return nil, nil, err
	}
	rt.Logger().Info("observability: reading traces from ClickHouse", "url", url)
	user := rt.Config("clickhouse.cleaner-user")
	if user == "" {
		rt.Logger().Warn("observability: no ClickHouse cleaner login (MODULE_OBSERVABILITY_CLICKHOUSE_CLEANER_USER); a deleted agent's spans stay until they expire")
		return traces, nil, nil
	}
	cleaning := cfg
	cleaning.User, cleaning.Password = user, rt.Config("clickhouse.cleaner-password")
	cleaner, err := clickhouse.New(cleaning)
	if err != nil {
		return nil, nil, err
	}
	return traces, cleaner, nil
}

var _ modkit.HealthReporter = module{}

// Health reports the trace store. Nothing writes to the in-memory one, so
// without ClickHouse the traces page stays empty.
func (module) Health(_ context.Context, rt modkit.Runtime) map[string]any {
	if rt.Config("clickhouse.url") == "" {
		return map[string]any{
			"store":  "memory",
			"status": "degraded, no ClickHouse configured (MODULE_OBSERVABILITY_CLICKHOUSE_URL), so no traces are stored",
		}
	}
	return map[string]any{"store": "clickhouse"}
}

// ExternalSchema is the ClickHouse schema: applied by `amp migrate` as the
// migrator login, and read as the reader login when the module is installed
// or enabled.
func (module) ExternalSchema(config func(string) string, apply bool) (modkit.ExternalSchema, error) {
	url := config("clickhouse.url")
	if url == "" {
		return nil, fmt.Errorf("%w: no ClickHouse (MODULE_OBSERVABILITY_CLICKHOUSE_URL)", modkit.ErrNotConfigured)
	}
	cfg := clickhouse.SchemaConfig{URL: url, Database: config("clickhouse.database")}
	if apply {
		cfg.User, cfg.Password = config("clickhouse.migrator-user"), config("clickhouse.migrator-password")
		if cfg.User == "" {
			return nil, fmt.Errorf("%w: no ClickHouse migrator login (MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_USER)", modkit.ErrNotConfigured)
		}
		days, err := ttlDays(config("clickhouse.ttl-days"))
		if err != nil {
			return nil, err
		}
		cfg.TTLDays = days
	} else {
		cfg.User, cfg.Password = config("clickhouse.user"), config("clickhouse.password")
		if cfg.User == "" {
			return nil, errNoReader
		}
	}
	s, err := clickhouse.NewSchema(cfg, clickhouse.Migrations())
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ttlDays is the configured span retention in days.
func ttlDays(v string) (int, error) {
	if v == "" {
		return defaultTTLDays, nil
	}
	days, err := strconv.Atoi(v)
	if err != nil || days <= 0 {
		return 0, fmt.Errorf("observability: clickhouse.ttl-days must be a positive number of days, not %q", v)
	}
	return days, nil
}

// cleanup deletes a deleted agent's spans.
type cleanup struct {
	cleaner agentCleaner
	log     *slog.Logger
}

// agentDeleted handles agent.deleted. Deleting spans already gone succeeds,
// so a retry is safe; an event it can never act on is logged and dropped.
func (c cleanup) agentDeleted(ctx context.Context, e modkit.Event) error {
	var agent struct {
		ID    string `json:"id"`
		OrgID string `json:"orgId"`
	}
	if err := json.Unmarshal(e.Data, &agent); err != nil {
		c.log.Error("observability: unreadable agent.deleted event", "event", e.ID, "error", err)
		return nil
	}
	org := e.OrgID
	if org == "" {
		org = agent.OrgID
	}
	if org == "" || agent.ID == "" {
		c.log.Error("observability: agent.deleted event names no org or agent", "event", e.ID)
		return nil
	}
	if c.cleaner == nil {
		c.log.Info("observability: not deleting a deleted agent's spans: no cleaner login", "org", org, "agent", agent.ID)
		return nil
	}
	if err := c.cleaner.DeleteAgent(ctx, org, agent.ID); err != nil {
		return fmt.Errorf("observability: deleting agent %s's spans: %w", agent.ID, err)
	}
	return nil
}
