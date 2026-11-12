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

//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestAgentVariableOwnership_SerializesAndReadsCommittedNames(t *testing.T) {
	database := db.GetDB()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ou := newOrg(t)
	_, proxyID := seedLLMProxy(t, ou)
	configID, envID := mapLLMProxyToAgent(t, ou, proxyID)
	require.NoError(t, database.Create(&models.AgentEnvConfigVariable{
		ConfigUUID: configID, EnvironmentUUID: envID, VariableKey: "url", OrgID: ou, VariableName: "OLD_URL",
		CreatedAt: time.Now().UTC(),
	}).Error)
	begin := func() *gorm.DB {
		tx := database.WithContext(ctx).Begin()
		require.NoError(t, tx.Error)
		t.Cleanup(func() { tx.Rollback() })
		return tx
	}
	repo := NewAgentEnvConfigVariableRepository(database)
	first := begin()
	rows, err := repo.LockAndListByAgent(ctx, first, ou, "checkout", "proxy-owner")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "OLD_URL", rows[0].VariableName)

	type outcome struct {
		rows []models.AgentEnvConfigVariable
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		// Begun here, not on the test goroutine: on SQLite beginning already waits for the owner's transaction.
		second := database.WithContext(ctx).Begin()
		if second.Error != nil {
			done <- outcome{err: second.Error}
			return
		}
		defer second.Rollback()
		got, readErr := repo.LockAndListByAgent(ctx, second, ou, "checkout", "proxy-owner")
		done <- outcome{rows: got, err: readErr}
	}()
	select {
	case got := <-done:
		t.Fatalf("a competing rename must wait for the owner transaction, got %v", got)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, first.Model(&models.AgentEnvConfigVariable{}).Where("config_id = ?", configID).
		Update("variable_name", "CLAIMED_URL").Error)
	require.NoError(t, first.Commit().Error)
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.Len(t, got.rows, 1)
		require.Equal(t, "CLAIMED_URL", got.rows[0].VariableName, "ownership read must see the winning rename, not the pre-lock snapshot")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
