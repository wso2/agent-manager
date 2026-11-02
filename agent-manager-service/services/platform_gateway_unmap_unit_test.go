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

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// stubGuard answers ServesEnvironment with serves and err, and records the gateway and environment it was asked about.
type stubGuard struct {
	serves       bool
	err          error
	env, gateway uuid.UUID
}

func (g *stubGuard) ServesEnvironment(_ context.Context, _ *gorm.DB, env, gateway uuid.UUID) (bool, error) {
	g.env, g.gateway = env, gateway
	return g.serves, g.err
}

// unmapRepo deletes the mapping inside its transaction; rolledBack reports whether that transaction failed.
func unmapRepo(rolledBack *bool) *repomocks.GatewayRepositoryMock {
	repo := &repomocks.GatewayRepositoryMock{
		DeleteEnvironmentMappingFunc: func(string, string) error { return nil },
	}
	repo.TransactionFunc = func(fn func(tx *gorm.DB) error) error {
		err := fn(nil)
		*rolledBack = err != nil
		return err
	}
	repo.WithTxFunc = func(*gorm.DB) repositories.GatewayRepository { return repo }
	return repo
}

func TestRemoveGatewayFromEnvironmentIsRefusedWhileTheGatewayServesTheEnvironment(t *testing.T) {
	gateway, env := uuid.New(), uuid.New()
	var rolledBack bool
	guard := &stubGuard{serves: true}
	svc := NewPlatformGatewayService(unmapRepo(&rolledBack), nil).WithEnvironmentGuard(guard)

	err := svc.RemoveGatewayFromEnvironment(context.Background(), gateway.String(), env.String())

	require.ErrorIs(t, err, utils.ErrGatewayServesEnvironment)
	assert.True(t, rolledBack, "the mapping delete is rolled back")
	assert.Equal(t, env, guard.env)
	assert.Equal(t, gateway, guard.gateway)
}

func TestRemoveGatewayFromEnvironmentRemovesAMappingTheGatewayDoesNotServe(t *testing.T) {
	var rolledBack bool
	svc := NewPlatformGatewayService(unmapRepo(&rolledBack), nil).WithEnvironmentGuard(&stubGuard{})

	err := svc.RemoveGatewayFromEnvironment(context.Background(), uuid.NewString(), uuid.NewString())

	require.NoError(t, err)
	assert.False(t, rolledBack)
}

func TestRemoveGatewayFromEnvironmentDoesNotReportAFailedCheckAsServing(t *testing.T) {
	var rolledBack bool
	checkErr := errors.New("connection refused")
	svc := NewPlatformGatewayService(unmapRepo(&rolledBack), nil).WithEnvironmentGuard(&stubGuard{err: checkErr})

	err := svc.RemoveGatewayFromEnvironment(context.Background(), uuid.NewString(), uuid.NewString())

	require.ErrorIs(t, err, checkErr)
	assert.NotErrorIs(t, err, utils.ErrGatewayServesEnvironment)
	assert.True(t, rolledBack)
}
