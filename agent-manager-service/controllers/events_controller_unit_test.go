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

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// describeFailingSubscriptions saves and reads subscriptions, but cannot load
// their destination and counts.
type describeFailingSubscriptions struct {
	services.EventSubscriptionService
	saved *models.EventSubscription
}

func (s *describeFailingSubscriptions) Create(
	_ context.Context, _ repositories.EventTarget, _ services.EventSubscriptionInput,
) (*models.EventSubscription, error) {
	return s.saved, nil
}

func (s *describeFailingSubscriptions) Get(
	context.Context, repositories.EventTarget, uuid.UUID,
) (*models.EventSubscription, error) {
	return s.saved, nil
}

func (s *describeFailingSubscriptions) Describe(
	context.Context, repositories.EventTarget, *models.EventSubscription,
) (*services.SubscriptionDetail, error) {
	return nil, errors.New("database unavailable")
}

func TestEventsController_SavedSubscriptionSurvivesDescribeFailure(t *testing.T) {
	saved := &models.EventSubscription{
		ID: uuid.New(), Scope: models.EventScopeOrg, Name: "Alerts", DestinationID: uuid.New(),
		EventTypes: []string{"agent.deployed"}, Environments: []string{}, Enabled: true,
	}
	h := NewEventsController(nil, &describeFailingSubscriptions{saved: saved}).ForScope(models.EventScopeOrg)

	body := `{"name":"Alerts","destinationId":"` + saved.DestinationID.String() + `","eventTypes":["agent.deployed"]}`
	w := httptest.NewRecorder()
	h.CreateSubscription(w, httptest.NewRequest(http.MethodPost, "/orgs/acme/events/subscriptions", strings.NewReader(body)))
	require.Equal(t, http.StatusCreated, w.Code, "the subscription is saved; a 500 would make a retry create it twice")
	var resp spec.EventSubscriptionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, saved.ID.String(), resp.Id)
	assert.Equal(t, saved.DestinationID.String(), resp.Destination.Id)
	assert.Equal(t, "Unknown destination", resp.Destination.Name)

	r := httptest.NewRequest(http.MethodGet, "/orgs/acme/events/subscriptions/"+saved.ID.String(), nil)
	r.SetPathValue(PathParamEventSubscriptionID, saved.ID.String())
	w = httptest.NewRecorder()
	h.GetSubscription(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code, "a read with nothing saved still reports the failure")
}
