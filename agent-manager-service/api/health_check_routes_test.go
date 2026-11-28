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

package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/sqlitedialect"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

// checkedVault is a local vault whose Check returns err.
type checkedVault struct {
	vault.Vault
	err error
}

func (c checkedVault) Check(context.Context) error { return c.err }

func healthStatus(t *testing.T, g *gorm.DB, vaults *vault.Vaults) int {
	t.Helper()
	mux := http.NewServeMux()
	registerHealthCheck(mux, vaults)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req = req.WithContext(db.CtxWithTx(req.Context(), g))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestHealthCheckReportsTheDatabaseAndTheVault(t *testing.T) {
	g, err := gorm.Open(sqlitedialect.Open("file:"+filepath.Join(t.TempDir(), "h.db")), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	local, err := vault.NewLocal(make([]byte, vault.KeySize))
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, healthStatus(t, g, nil))
	require.Equal(t, http.StatusOK, healthStatus(t, g, vault.Of(checkedVault{Vault: local})))
	require.Equal(t, http.StatusServiceUnavailable,
		healthStatus(t, g, vault.Of(checkedVault{Vault: local, err: errors.New("sealed")})))

	sqlDB, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	require.Equal(t, http.StatusInternalServerError, healthStatus(t, g, nil))
}
