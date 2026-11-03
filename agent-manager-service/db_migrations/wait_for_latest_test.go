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

package dbmigrations

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitFor_TimesOutBeforeAnyMigration(t *testing.T) {
	g := scratchDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := waitFor(ctx, g, 1, 50*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitFor_TimesOutWhileBehindTarget(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 1))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := waitFor(ctx, g, 2, 50*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitFor_ReturnsOnceTargetIsApplied(t *testing.T) {
	g := scratchDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- waitFor(ctx, g, 2, 50*time.Millisecond) }()

	select {
	case err := <-done:
		t.Fatalf("returned before the migration ran: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	require.NoError(t, migrateTo(g, 2))
	assert.NoError(t, <-done)
}

func TestWaitFor_AcceptsANewerSchema(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 3))

	assert.NoError(t, waitFor(context.Background(), g, 2, 50*time.Millisecond))
}
