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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestDeploymentTimeoutSweeperFailsTransitionsOlderThanTheTimeout(t *testing.T) {
	repo := &repomocks.DeploymentRepositoryMock{
		FailStaleTransitionalFunc: func(context.Context, time.Time) (int64, error) { return 2, nil },
	}
	sweeper := NewDeploymentTimeoutSweeper(repo, discardLogger(), time.Minute, 5*time.Minute)

	before := time.Now()
	sweeper.RunOnce(context.Background())

	calls := repo.FailStaleTransitionalCalls()
	require.Len(t, calls, 1)
	assert.WithinDuration(t, before.Add(-5*time.Minute), calls[0].OlderThan, time.Second)
}

func TestDeploymentTimeoutSweeperKeepsSweepingAfterAnError(t *testing.T) {
	swept := make(chan struct{}, 2)
	repo := &repomocks.DeploymentRepositoryMock{
		FailStaleTransitionalFunc: func(context.Context, time.Time) (int64, error) {
			swept <- struct{}{}
			return 0, errors.New("db down")
		},
	}
	sweeper := NewDeploymentTimeoutSweeper(repo, discardLogger(), 10*time.Millisecond, 5*time.Minute)
	require.NoError(t, sweeper.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, sweeper.Stop()) })

	for range 2 {
		select {
		case <-swept:
		case <-time.After(time.Second):
			t.Fatal("the sweeper stopped sweeping")
		}
	}
}

func TestDeploymentTimeoutSweeperStopsWithItsContext(t *testing.T) {
	repo := &repomocks.DeploymentRepositoryMock{
		FailStaleTransitionalFunc: func(context.Context, time.Time) (int64, error) { return 0, nil },
	}
	sweeper := NewDeploymentTimeoutSweeper(repo, discardLogger(), 10*time.Millisecond, 5*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, sweeper.Start(ctx))
	cancel()
	require.NoError(t, sweeper.Stop())
	require.NoError(t, sweeper.Stop(), "Stop is idempotent")
}
