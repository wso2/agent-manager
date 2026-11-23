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

package modhost_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
)

// A module that requires notifications/v1 installs (the core provides it) and may hold a
// validate hook on event destinations, which the core runs before a rename or delete.
func TestEventDestinationHooksAndNotifications(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mf := manifest("t-chan", []string{notifyv1.ID}, nil)
	var asked []modkit.HookRequest
	m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.Hook(modkit.ResourceEventDestination, modkit.Validate, func(_ context.Context, req modkit.HookRequest) error {
			asked = append(asked, req)
			if req.Object["name"] == "in-use" {
				return &modkit.Error{Status: http.StatusConflict, Message: "used"}
			}
			return nil
		})
		return nil
	}}
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
	installAll(t, h, "t-chan")
	assert.Contains(t, modhost.CoreCapabilities, notifyv1.ID)

	run := func(name string) error {
		return h.Begin(ctx, modkit.ResourceEventDestination).Run(ctx, nil, modkit.Validate, modkit.HookRequest{
			Action: modkit.ActionDelete, OrgID: "o", ID: "d", Object: map[string]any{"name": name},
		}, nil)
	}
	require.NoError(t, run("free"))
	var rejected *modkit.Error
	require.ErrorAs(t, run("in-use"), &rejected)
	assert.Equal(t, http.StatusConflict, rejected.Status)
	require.Len(t, asked, 2)
	assert.Equal(t, modkit.ResourceEventDestination, asked[0].Resource)
}

func TestEventDestinationsTakeOnlyValidateHooks(t *testing.T) {
	for _, phase := range []modkit.Phase{modkit.Apply, modkit.Finalize} {
		t.Run(string(phase), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m := &testModule{manifest: manifest("t-bad", nil, nil), activate: func(rt modkit.Runtime) error {
				rt.Hook(modkit.ResourceEventDestination, phase, func(context.Context, modkit.HookRequest) error { return nil })
				return nil
			}}
			h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
			_, err := h.Install(ctx, "t-bad", false)
			require.Error(t, err)
			assert.Empty(t, h.Active())
		})
	}
}
