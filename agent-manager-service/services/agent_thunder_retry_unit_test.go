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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func bindingWith(status models.AgentThunderStatus) *models.AgentThunderClient {
	return &models.AgentThunderClient{
		ID: uuid.New(), OUID: "ou", ProjectName: "p", AgentName: "a", EnvironmentName: "default",
		ProvisioningType: models.AgentProvisioningTypeExternal, Status: status,
		AttemptCount: 5, LastError: "thunder down", ThunderAgentID: "tid", RequestedBy: "user-1",
	}
}

func TestRetryProvisioning_ResetsAFailedBindingAndAttemptsAgain(t *testing.T) {
	// bindingWith carries the last attempt's error.
	waiting := bindingWith(models.AgentThunderStatusPending)
	for name, failed := range map[string]*models.AgentThunderClient{
		"failed": bindingWith(models.AgentThunderStatusFailed),
		// A failed attempt with attempts left waits for its automatic retry
		// with an error that asks the user to retry.
		"pending with an error": waiting,
	} {
		t.Run(name, func(t *testing.T) {
			attempted := make(chan uuid.UUID, 1)
			repo := &repomocks.AgentThunderClientRepositoryMock{
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
					return failed, nil
				},
				ResetFailedForRetryFunc: func(_ context.Context, id uuid.UUID) (bool, error) {
					assert.Equal(t, failed.ID, id)
					return true, nil
				},
				// The background attempt starts by claiming the binding; declining ends it there.
				ClaimForAttemptFunc: func(_ context.Context, id uuid.UUID) (bool, error) {
					attempted <- id
					return false, nil
				},
			}
			svc := newTestProvisioningService(repo, nil, nil)

			view, err := svc.RetryProvisioning(context.Background(), "ou", "p", "a", "default")
			require.NoError(t, err)
			assert.Equal(t, models.AgentThunderStatusPending, view.Status)
			assert.Equal(t, "default", view.EnvironmentName)
			assert.Equal(t, "tid", view.AgentID)
			assert.Equal(t, "user-1", view.RequestedBy)
			select {
			case id := <-attempted:
				assert.Equal(t, failed.ID, id)
			case <-time.After(5 * time.Second):
				t.Fatal("no provisioning attempt followed the reset")
			}
		})
	}
}

func TestRetryProvisioning_Refusals(t *testing.T) {
	errDB := errors.New("db down")
	for name, tc := range map[string]struct {
		get   func() (*models.AgentThunderClient, error)
		reset func() (bool, error)
		want  error
	}{
		"no binding": {
			get:  func() (*models.AgentThunderClient, error) { return nil, repositories.ErrAgentThunderClientNotFound },
			want: utils.ErrAgentIdentityNotProvisioned,
		},
		"read fails": {
			get:  func() (*models.AgentThunderClient, error) { return nil, errDB },
			want: errDB,
		},
		"not failed": {
			get: func() (*models.AgentThunderClient, error) {
				return bindingWith(models.AgentThunderStatusCompleted), nil
			},
			want: utils.ErrAgentIdentityRetryNotAllowed,
		},
		"pending without an error": {
			get: func() (*models.AgentThunderClient, error) {
				b := bindingWith(models.AgentThunderStatusPending)
				b.LastError = ""
				return b, nil
			},
			want: utils.ErrAgentIdentityRetryNotAllowed,
		},
		"lost the race": {
			get:   func() (*models.AgentThunderClient, error) { return bindingWith(models.AgentThunderStatusFailed), nil },
			reset: func() (bool, error) { return false, nil },
			want:  utils.ErrAgentIdentityRetryNotAllowed,
		},
		"reset fails": {
			get:   func() (*models.AgentThunderClient, error) { return bindingWith(models.AgentThunderStatusFailed), nil },
			reset: func() (bool, error) { return false, errDB },
			want:  errDB,
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &repomocks.AgentThunderClientRepositoryMock{
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
					return tc.get()
				},
			}
			if tc.reset != nil {
				repo.ResetFailedForRetryFunc = func(context.Context, uuid.UUID) (bool, error) { return tc.reset() }
			}
			// ClaimForAttemptFunc is unset: no attempt may start after a refusal.
			_, err := newTestProvisioningService(repo, nil, nil).RetryProvisioning(context.Background(), "ou", "p", "a", "default")
			require.ErrorIs(t, err, tc.want)
			if name == "read fails" {
				assert.NotErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned, "an outage is not reported as not provisioned")
			}
		})
	}
}

func TestGetBindingState(t *testing.T) {
	ctx := context.Background()
	b := bindingWith(models.AgentThunderStatusFailed)
	b.SecretRefPath = "ref"
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, agent, _ string) (*models.AgentThunderClient, error) {
			switch agent {
			case "a":
				return b, nil
			case "none":
				return nil, repositories.ErrAgentThunderClientNotFound
			}
			return nil, errors.New("db down")
		},
	}
	svc := newTestProvisioningService(repo, nil, nil)

	state, err := svc.GetBindingState(ctx, "ou", "p", "a", "default")
	require.NoError(t, err)
	assert.Equal(t, &AgentThunderBindingState{
		ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusFailed,
		HasSecret: true, LastError: "thunder down",
	}, state)

	state, err = svc.GetBindingState(ctx, "ou", "p", "none", "default")
	require.NoError(t, err)
	assert.Nil(t, state, "no binding yet is not an error")

	_, err = svc.GetBindingState(ctx, "ou", "p", "broken", "default")
	require.ErrorContains(t, err, "db down")
}
