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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
)

func TestWorkloadAdmissionBeforeOpenChoreo(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("suspended work reached OpenChoreo") }))
	ctx := workloadpolicy.WithGuard(context.Background(), func(_ context.Context, org string) error {
		require.Equal(t, "org-a", org)
		return workloadpolicy.ErrSuspended
	})
	require.ErrorIs(t, c.Deploy(ctx, "org-a", "proj", "agent", DeployRequest{}), workloadpolicy.ErrSuspended)
	require.ErrorIs(t, c.EnsureReleaseAndBinding(ctx, "org-a", "proj", "agent", "dev", nil, nil, nil, nil), workloadpolicy.ErrSuspended)
	require.ErrorIs(t, c.PromoteComponent(ctx, "org-a", "proj", "agent", "dev", "prod", nil, nil, nil, nil), workloadpolicy.ErrSuspended)
	require.ErrorIs(t, c.UpdateDeploymentState(ctx, "org-a", "proj", "agent", "dev", gen.ReleaseBindingSpecStateActive), workloadpolicy.ErrSuspended)
	_, err := c.TriggerBuild(ctx, "org-a", "proj", "agent", "commit", "", nil)
	require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
	_, err = c.CreateWorkflowRun(ctx, "org-a", CreateWorkflowRunRequest{})
	require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
	require.ErrorIs(t, c.CreateComponent(ctx, "org-a", "proj", CreateComponentRequest{}), workloadpolicy.ErrSuspended)
}

func TestDeploymentStateSuspensionMarker(t *testing.T) {
	for _, state := range []gen.ReleaseBindingSpecState{gen.ReleaseBindingSpecStateActive, gen.ReleaseBindingSpecStateUndeploy} {
		t.Run(string(state), func(t *testing.T) {
			wrote := false
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					_, err := w.Write([]byte(`{"items":[{"metadata":{"name":"agent-dev","annotations":{"cloud.wso2.com/lifecycle-suspended-at":"2026-11-05T00:00:00Z"}},"spec":{"environment":"dev","state":"Undeploy","owner":{"componentName":"agent","projectName":"proj"}}}]}`))
					require.NoError(t, err)
				case http.MethodPut:
					wrote = true
					var binding gen.ReleaseBinding
					require.NoError(t, json.NewDecoder(r.Body).Decode(&binding))
					require.NotContains(t, *binding.Metadata.Annotations, "cloud.wso2.com/lifecycle-suspended-at")
					require.Equal(t, gen.ReleaseBindingSpecStateUndeploy, *binding.Spec.State)
					_, err := w.Write([]byte(`{}`))
					require.NoError(t, err)
				}
			}))
			ctx := workloadpolicy.WithGuard(context.Background(), func(context.Context, string) error { return workloadpolicy.ErrSuspended })
			// No context guard exercises the annotation's independent defense on Start.
			if state == gen.ReleaseBindingSpecStateActive {
				ctx = context.Background()
			}
			err := c.UpdateDeploymentState(ctx, "org-a", "proj", "agent", "dev", state)
			if state == gen.ReleaseBindingSpecStateActive {
				require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
				require.False(t, wrote)
			} else {
				require.NoError(t, err)
				require.True(t, wrote)
			}
		})
	}
}

func TestDeployMutatorCannotClearSuspension(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"metadata":{"name":"agent-dev","annotations":{"cloud.wso2.com/lifecycle-suspended-at":"2026-11-05T00:00:00Z"}},"spec":{"state":"Undeploy","environment":"dev","owner":{"componentName":"agent","projectName":"proj"}}}`))
		require.NoError(t, err)
	}))
	err := c.retryReleaseBindingUpdate(context.Background(), "default", "agent-dev", func(b *gen.ReleaseBinding) { state := gen.ReleaseBindingSpecStateActive; b.Spec.State = &state })
	require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
}
