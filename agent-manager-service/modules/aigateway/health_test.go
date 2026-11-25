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

package aigateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// healthRuntime is the part of the runtime Health uses.
type healthRuntime struct {
	modkit.Runtime
	db *gorm.DB
}

func (r healthRuntime) DB() *gorm.DB       { return r.db }
func (healthRuntime) Config(string) string { return "" }

// Health reads connections from the gateway table, so every replica gives the
// same answer, and no gateway connected is ok.
func TestHealthReportsGatewayConnections(t *testing.T) {
	tx := db.GetDB().Begin()
	t.Cleanup(func() { tx.Rollback() })
	rt := healthRuntime{db: tx}

	// Other tests may have left gateways; count from what is there.
	detail := module{}.Health(context.Background(), rt)
	require.NotContains(t, detail, "error")
	total, connected := detail["gateways"].(int64), detail["connected"].(int64)
	if connected == 0 {
		assert.Equal(t, "ok, no gateway connected", detail["status"])
	}

	org := dbtest.Org(t, tx)
	up := gwtest.Gateway(t, tx, org, "ingress")
	down := gwtest.Gateway(t, tx, org, "ingress")
	require.NoError(t, tx.Model(&gwmodels.Gateway{}).Where("id = ?", down.UUID).Update("is_active", false).Error)

	// Other packages share the database and may add gateways between the
	// two counts, so the totals only grow by at least ours; the per-gateway
	// check below is exact.
	detail = module{}.Health(context.Background(), rt)
	assert.GreaterOrEqual(t, detail["gateways"].(int64), total+2)
	assert.GreaterOrEqual(t, detail["connected"].(int64), connected+1)
	assert.NotContains(t, detail, "status")
	got := map[string]bool{}
	for _, g := range detail["registered"].([]gatewayHealth) {
		if g.ID == up.UUID.String() || g.ID == down.UUID.String() {
			got[g.ID] = g.Connected
		}
	}
	assert.Equal(t, map[string]bool{up.UUID.String(): true, down.UUID.String(): false}, got)
	assert.NotNil(t, detail["failureSummary"])
}
