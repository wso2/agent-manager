// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
)

// modeProvisioning reports an identity management mode and nothing else.
type modeProvisioning struct {
	AgentThunderProvisioningService
	mode thundersvc.IdentityManagementMode
}

func (m modeProvisioning) IdentityManagementMode() thundersvc.IdentityManagementMode { return m.mode }

type initialBuildCtxKey struct{}

func newInitialBuildService(mode thundersvc.IdentityManagementMode) *agentManagerService {
	return &agentManagerService{
		logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		agentThunderProvisioning: modeProvisioning{mode: mode},
	}
}

// Under CP_MANAGED identity the first build is triggered in the background, so a slow GitHub or
// OpenChoreo call cannot hold the create request past the edge route's timeout.
func TestStartInitialBuildDoesNotHoldTheCreateRequestInCPManagedMode(t *testing.T) {
	svc := newInitialBuildService(thundersvc.IdentityManagementCPManaged)
	release := make(chan struct{})
	type seen struct {
		err   error
		value any
	}
	finished := make(chan seen, 1)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), initialBuildCtxKey{}, "caller"))

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		svc.startInitialBuild(ctx, "agent-a", func(buildCtx context.Context) error {
			<-release
			finished <- seen{err: buildCtx.Err(), value: buildCtx.Value(initialBuildCtxKey{})}
			return nil
		})
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		close(release) // let the stuck trigger go, then fail
		t.Fatal("the create request waited for the build trigger")
	}

	// The request ends (the edge cancels it) while the trigger is still running.
	cancel()
	close(release)
	select {
	case got := <-finished:
		require.NoError(t, got.err, "the request's cancellation must not reach the trigger")
		require.Equal(t, "caller", got.value, "the caller's values must reach the trigger")
	case <-time.After(5 * time.Second):
		t.Fatal("the background trigger never finished")
	}
}

// Under LEGACY_DIRECT the first environment's AgentID env vars ride on this run, so it stays inline.
func TestStartInitialBuildStaysInlineInLegacyDirectMode(t *testing.T) {
	svc := newInitialBuildService(thundersvc.IdentityManagementLegacyDirect)
	ran := false
	svc.startInitialBuild(context.Background(), "agent-a", func(context.Context) error {
		ran = true
		return nil
	})
	require.True(t, ran, "the trigger must have finished when the call returns")
}

// A failed trigger is logged and never fails the create.
func TestStartInitialBuildFailureIsNotFatal(t *testing.T) {
	svc := newInitialBuildService(thundersvc.IdentityManagementLegacyDirect)
	require.NotPanics(t, func() {
		svc.startInitialBuild(context.Background(), "agent-a", func(context.Context) error { return errors.New("github down") })
	})
}
