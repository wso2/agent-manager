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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type imageCleanupCall struct {
	ouID, projectName, componentName string
	ctxErr                           error
}

// recordingImageCleaner reports each DeleteComponentImages call on a channel, since the
// agent delete makes it in the background.
type recordingImageCleaner struct {
	calls chan imageCleanupCall
	err   error
}

func newRecordingImageCleaner(err error) *recordingImageCleaner {
	return &recordingImageCleaner{calls: make(chan imageCleanupCall, 4), err: err}
}

func (c *recordingImageCleaner) DeleteComponentImages(ctx context.Context, ouID, projectName, componentName string) error {
	c.calls <- imageCleanupCall{ouID: ouID, projectName: projectName, componentName: componentName, ctxErr: ctx.Err()}
	return c.err
}

func (c *recordingImageCleaner) requireCall(t *testing.T) imageCleanupCall {
	t.Helper()
	select {
	case call := <-c.calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("the agent's build images were not cleaned up")
		return imageCleanupCall{}
	}
}

func (c *recordingImageCleaner) requireNoCall(t *testing.T) {
	t.Helper()
	select {
	case call := <-c.calls:
		t.Fatalf("unexpected image cleanup for %s/%s", call.projectName, call.componentName)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestDeleteAgentCleansUpBuildImages(t *testing.T) {
	cleaner := newRecordingImageCleaner(nil)
	manager := deletionManager(deletableAgentOC(func(context.Context) error { return nil }), nil)
	manager.SetComponentImageCleaner(cleaner)

	callerCtx, cancelCaller := context.WithCancel(auditableCtx(t))
	_, err := manager.DeleteAgent(callerCtx, "org-a", "project-a", "agent-a")
	require.NoError(t, err)
	// The caller leaving once the delete returns must not cancel the cleanup.
	cancelCaller()

	call := cleaner.requireCall(t)
	require.Equal(t, imageCleanupCall{ouID: "org-a", projectName: "project-a", componentName: "agent-a"}, call)
}

func TestDeleteAgentCleansUpBuildImagesWhenTheComponentIsAlreadyGone(t *testing.T) {
	cleaner := newRecordingImageCleaner(nil)
	manager := deletionManager(deletableAgentOC(func(context.Context) error { return utils.ErrNotFound }), nil)
	manager.SetComponentImageCleaner(cleaner)

	_, err := manager.DeleteAgent(auditableCtx(t), "org-a", "project-a", "agent-a")
	require.NoError(t, err)

	require.Equal(t, "agent-a", cleaner.requireCall(t).componentName)
}

func TestDeleteAgentSucceedsWhenImageCleanupFails(t *testing.T) {
	cleaner := newRecordingImageCleaner(errors.New("registry unavailable"))
	manager := deletionManager(deletableAgentOC(func(context.Context) error { return nil }), nil)
	manager.SetComponentImageCleaner(cleaner)

	_, err := manager.DeleteAgent(auditableCtx(t), "org-a", "project-a", "agent-a")
	require.NoError(t, err)
	cleaner.requireCall(t)
}

func TestDeleteAgentSkipsImageCleanupForExternalAgents(t *testing.T) {
	cleaner := newRecordingImageCleaner(nil)
	oc := deletableAgentOC(func(context.Context) error { return nil })
	oc.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.ExternalAgent)}}, nil
	}
	manager := deletionManager(oc, nil)
	manager.SetComponentImageCleaner(cleaner)

	_, err := manager.DeleteAgent(auditableCtx(t), "org-a", "project-a", "agent-a")
	require.NoError(t, err)
	cleaner.requireNoCall(t)
}

func TestDeleteAgentSkipsImageCleanupWhenTheDeleteFails(t *testing.T) {
	cleaner := newRecordingImageCleaner(nil)
	manager := deletionManager(deletableAgentOC(func(context.Context) error { return errors.New("openchoreo unavailable") }), nil)
	manager.SetComponentImageCleaner(cleaner)

	_, err := manager.DeleteAgent(auditableCtx(t), "org-a", "project-a", "agent-a")
	require.Error(t, err)
	cleaner.requireNoCall(t)
}

// Without a cleaner (the open-source default) the delete behaves as before.
func TestDeleteAgentWithoutImageCleaner(t *testing.T) {
	oc := deletableAgentOC(func(context.Context) error { return nil })
	manager := deletionManager(oc, nil)

	_, err := manager.DeleteAgent(auditableCtx(t), "org-a", "project-a", "agent-a")
	require.NoError(t, err)
	require.Len(t, oc.DeleteComponentCalls(), 1)
}
