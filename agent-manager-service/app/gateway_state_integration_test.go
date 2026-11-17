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

package app

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/agentgwv1"
)

// With no gateway linked, a database holding gateway state is reported; with
// the gateway's capability active, nothing is.
func TestWarnGatewayStateWithoutProvider(t *testing.T) {
	ctx := context.Background()
	g := db.GetDB()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbtest.Gateway(t, g, dbtest.Org(t, g), models.GatewayRoleEgress)

	coreext.Set(nil)
	held := warnGatewayStateWithoutProvider(ctx, g, log)
	require.True(t, slices.ContainsFunc(held, func(s string) bool { return strings.HasPrefix(s, "gateways=") }), held)

	coreext.Set(coreext.Static(map[string]any{agentgwv1.ID: struct{}{}}))
	t.Cleanup(func() { coreext.Set(nil) })
	require.Empty(t, warnGatewayStateWithoutProvider(ctx, g, log))
}
