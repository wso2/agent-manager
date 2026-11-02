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
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Two concurrent assigns of one gateway to different environments take different environment
// locks, so only the per-gateway lock can keep the second one out. Real commits are needed
// for the loser to see the winner's mapping, hence no rolled-back test transaction here.
func TestAssignGatewayConcurrentEnvironmentsRefused(t *testing.T) {
	gdb := db.DB(context.Background())
	id := seedRuntimeURLGateway(t, gdb, "env-fixed-"+uuid.NewString()[:8])
	t.Cleanup(func() { require.NoError(t, gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, id).Error) })
	svc := services.NewPlatformGatewayService(repositories.NewGatewayRepo(gdb), nil)

	environments := []string{uuid.NewString(), uuid.NewString()}
	errs := make([]error, len(environments))
	var wg sync.WaitGroup
	for i, env := range environments {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = svc.AssignGatewayToEnvironment(id.String(), env)
		}()
	}
	wg.Wait()

	refused := 0
	for _, err := range errs {
		if err != nil {
			require.ErrorIs(t, err, utils.ErrGatewayEnvironmentFixed)
			refused++
		}
	}
	require.Equal(t, 1, refused, "exactly one assign must be refused; errors: %v", errs)
	var mappings int64
	require.NoError(t, gdb.Raw(`SELECT count(*) FROM gateway_environment_mappings WHERE gateway_uuid = ?`, id).Row().Scan(&mappings))
	require.EqualValues(t, 1, mappings)
}
