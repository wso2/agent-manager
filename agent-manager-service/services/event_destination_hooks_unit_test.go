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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// channelHooks is a module host whose event-destination validate hook refuses the
// destinations named in inUse, and records what it was asked.
type channelHooks struct {
	inUse map[string]bool
	asked *[]modkit.HookRequest
}

func (h channelHooks) Begin(_ context.Context, resource string) coreext.Operation {
	return channelOp{h: h, resource: resource}
}
func (channelHooks) Capability(string) (any, bool) { return nil, false }

type channelOp struct {
	h        channelHooks
	resource string
}

func (channelOp) Check(map[string]json.RawMessage) error { return nil }
func (channelOp) Emit(context.Context, *gorm.DB, modkit.Event) error {
	return nil
}

func (o channelOp) Run(_ context.Context, _ *gorm.DB, phase modkit.Phase, req modkit.HookRequest, _ map[string]json.RawMessage) error {
	if o.resource != modkit.ResourceEventDestination || phase != modkit.Validate {
		return nil
	}
	*o.h.asked = append(*o.h.asked, req)
	if name, _ := req.Object["name"].(string); o.h.inUse[name] {
		return &modkit.Error{Status: http.StatusConflict, Message: "budget(s) b1 send warnings to it"}
	}
	return nil
}

// A module that names a channel refuses its rename and delete with the core's in-use
// error; edits that keep the name, and channels no module names, go ahead.
func TestEventDestinationService_ModulesCanRefuseRenameAndDelete(t *testing.T) {
	var asked []modkit.HookRequest
	coreext.Set(channelHooks{inUse: map[string]bool{"ops-hook": true}, asked: &asked})
	t.Cleanup(func() { coreext.Set(nil) })
	svc, _, repo := newTestDestinationService(&fakeDestinationDriver{})
	ctx := auditCtx(&recordingAuditor{})
	in := validDestinationInput()
	in.Name = "ops-hook"
	d, _, err := svc.Create(ctx, "ou-1", in)
	require.NoError(t, err)

	in.Description = "edited"
	_, err = svc.Update(ctx, "ou-1", d.ID, in)
	require.NoError(t, err, "an edit that keeps the name asks nobody")
	assert.Empty(t, asked)

	renamed := in
	renamed.Name = "ops-hook-2"
	_, err = svc.Update(ctx, "ou-1", d.ID, renamed)
	require.ErrorIs(t, err, utils.ErrEventDestinationInUse)
	assert.Contains(t, err.Error(), "budget(s) b1 send warnings to it")
	assert.NotContains(t, err.Error(), "subscriptions", "a module's refusal is worded by the module")
	require.Len(t, asked, 1)
	assert.Equal(t, modkit.ActionUpdate, asked[0].Action)
	assert.Equal(t, "ops-hook-2", asked[0].Object["newName"])

	err = svc.Delete(ctx, "ou-1", d.ID)
	require.ErrorIs(t, err, utils.ErrEventDestinationInUse)
	assert.Empty(t, repo.DeleteDestinationCalls(), "nothing is deleted while a module uses it")

	free := validDestinationInput()
	free.Name = "free-hook"
	other, _, err := svc.Create(ctx, "ou-1", free)
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, "ou-1", other.ID))
}
