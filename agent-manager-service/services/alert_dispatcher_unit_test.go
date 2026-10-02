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
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type dispatchOutcome struct {
	status   string
	attempts int
	next     time.Time
	health   []bool
}

func runDispatch(t *testing.T, delivery models.AlertDelivery, endpoint *models.AlertEndpoint, sender *fakeAlertSender) *dispatchOutcome {
	t.Helper()
	out := &dispatchOutcome{}
	repo := &repomocks.AlertRepositoryMock{
		ClaimDueDeliveriesFunc: func(context.Context, time.Time, time.Duration, int) ([]models.AlertDelivery, error) {
			return []models.AlertDelivery{delivery}, nil
		},
		GetEndpointFunc: func(context.Context, string) (*models.AlertEndpoint, error) {
			if endpoint == nil {
				return nil, gorm.ErrRecordNotFound
			}
			return endpoint, nil
		},
		MarkDeliverySentFunc: func(_ context.Context, _ uuid.UUID, attempts int, _ int, _ time.Time) error {
			out.status, out.attempts = models.AlertDeliveryStatusSent, attempts
			return nil
		},
		MarkDeliveryRetryFunc: func(_ context.Context, _ uuid.UUID, attempts int, _ *int, _ string, next time.Time) error {
			out.status, out.attempts, out.next = models.AlertDeliveryStatusPending, attempts, next
			return nil
		},
		MarkDeliveryDeadFunc: func(_ context.Context, _ uuid.UUID, attempts int, _ *int, _ string) error {
			out.status, out.attempts = models.AlertDeliveryStatusDead, attempts
			return nil
		},
		RecordEndpointOutcomeFunc: func(_ context.Context, _ string, success bool, _ time.Time) error {
			out.health = append(out.health, success)
			return nil
		},
	}
	d := NewAlertDispatcherService(repo, sender, testAlertKey, discardLogger())
	d.RunOnce(context.Background())
	return out
}

func testEndpoint(t *testing.T) *models.AlertEndpoint {
	t.Helper()
	secret, err := utils.EncryptBytes([]byte("whsec_x"), testAlertKey)
	require.NoError(t, err)
	return &models.AlertEndpoint{OUID: "org-1", URL: "https://alerts.example.com", Enabled: true, SigningSecretEncrypted: secret}
}

func testDelivery(attempts int) models.AlertDelivery {
	return models.AlertDelivery{ID: uuid.New(), OUID: "org-1", EventID: "evt-1", EventType: models.AlertEventMonitorRunFailed, Payload: "{}", Attempts: attempts}
}

func TestAlertDispatcher_Success(t *testing.T) {
	sender := &fakeAlertSender{code: 204}
	out := runDispatch(t, testDelivery(0), testEndpoint(t), sender)
	assert.Equal(t, models.AlertDeliveryStatusSent, out.status)
	assert.Equal(t, 1, out.attempts)
	assert.Equal(t, []bool{true}, out.health)
	require.Len(t, sender.targets, 1)
	assert.Equal(t, "whsec_x", sender.targets[0].SigningSecret)
}

func TestAlertDispatcher_TransientFailureRetries(t *testing.T) {
	before := time.Now()
	out := runDispatch(t, testDelivery(1), testEndpoint(t), &fakeAlertSender{code: 503, err: errors.New("unavailable")})
	assert.Equal(t, models.AlertDeliveryStatusPending, out.status)
	assert.Equal(t, 2, out.attempts)
	assert.WithinDuration(t, before.Add(alertRetryBackoff[1]), out.next, 5*time.Second)
	assert.Empty(t, out.health, "a retry is not a final outcome")
}

func TestAlertDispatcher_ClientErrorIsFinal(t *testing.T) {
	out := runDispatch(t, testDelivery(0), testEndpoint(t), &fakeAlertSender{code: 400, err: errors.New("bad request")})
	assert.Equal(t, models.AlertDeliveryStatusDead, out.status)
	assert.Equal(t, []bool{false}, out.health)
}

func TestAlertDispatcher_BudgetExhausted(t *testing.T) {
	out := runDispatch(t, testDelivery(len(alertRetryBackoff)), testEndpoint(t), &fakeAlertSender{err: errors.New("dial timeout")})
	assert.Equal(t, models.AlertDeliveryStatusDead, out.status)
	assert.Equal(t, len(alertRetryBackoff)+1, out.attempts)
}

func TestAlertDispatcher_EndpointRemoved(t *testing.T) {
	sender := &fakeAlertSender{code: 200}
	out := runDispatch(t, testDelivery(0), nil, sender)
	assert.Equal(t, models.AlertDeliveryStatusDead, out.status)
	assert.Empty(t, sender.sent, "nothing is sent without an endpoint")
}

func TestAlertDispatcher_LeaseCoversWholeBatch(t *testing.T) {
	// Deliveries in a claimed batch are sent sequentially; the lease must not
	// expire before the last one could be attempted.
	assert.Greater(t, alertDispatchClaimLease, time.Duration(alertDispatchBatch)*alertSendTimeout)
}
