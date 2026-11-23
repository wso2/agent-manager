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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// catalogDriver is a "multi" kind that delivers to several services.
type catalogDriver struct{ fakeDestinationDriver }

func (c *catalogDriver) Kind() string { return "multi" }

func (c *catalogDriver) Services() []eventdestinations.Service {
	return []eventdestinations.Service{{ID: "chat", Label: "Chat"}}
}

func TestEventDestinationService_Kinds(t *testing.T) {
	svc, _, _ := newTestDestinationService(&fakeDestinationDriver{hasSecret: true}, &credentialDriver{}, &catalogDriver{})
	kinds := svc.Kinds()
	require.Len(t, kinds, 3)
	byKind := map[string]DestinationKind{}
	for _, k := range kinds {
		byKind[k.Kind] = k
	}
	assert.Equal(t, []string{"cred", "fake", "multi"}, []string{kinds[0].Kind, kinds[1].Kind, kinds[2].Kind}, "sorted")
	assert.True(t, byKind["fake"].GeneratedSecret)
	assert.False(t, byKind["fake"].SuppliedCredential)
	assert.True(t, byKind["cred"].SuppliedCredential)
	assert.False(t, byKind["cred"].GeneratedSecret)
	assert.Equal(t, []eventdestinations.Service{{ID: "chat", Label: "Chat"}}, byKind["multi"].Services)
	assert.NotNil(t, byKind["fake"].Services, "an empty list, never null")
}

func TestEventDestinationService_AvailableToAProject(t *testing.T) {
	svc, store, _ := newTestDestinationService(&fakeDestinationDriver{})
	ou := "ou-avail"
	add := func(name, availability string, projects ...string) uuid.UUID {
		id := uuid.New()
		store.mu.Lock()
		store.destinations[id] = models.EventDestination{ID: id, OUID: ou, Name: name, Availability: availability, Projects: projects}
		store.mu.Unlock()
		return id
	}
	add("all", models.DestinationAvailableAll)
	add("payments-only", models.DestinationAvailableProjects, "payments")
	add("org-only", models.DestinationAvailableOrg)

	got, _, err := svc.Available(context.Background(), ou, "payments")
	require.NoError(t, err)
	names := []string{}
	for _, d := range got {
		names = append(names, d.Name)
	}
	assert.ElementsMatch(t, []string{"all", "payments-only"}, names)

	got, _, err = svc.Available(context.Background(), ou, "claims")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "all", got[0].Name)
}

func TestEventDestinationService_ReadErrors(t *testing.T) {
	ctx := context.Background()
	errDB := errors.New("db down")
	svc, _, repo := newTestDestinationService(&fakeDestinationDriver{})

	_, err := svc.Get(ctx, "ou", uuid.New())
	require.ErrorIs(t, err, utils.ErrEventDestinationNotFound)
	_, err = svc.Usage(ctx, "ou", uuid.New())
	require.ErrorIs(t, err, utils.ErrEventDestinationNotFound, "usage of a missing destination")

	repo.GetDestinationFunc = func(context.Context, string, uuid.UUID) (*models.EventDestination, error) {
		return nil, errDB
	}
	_, err = svc.Get(ctx, "ou", uuid.New())
	require.ErrorIs(t, err, errDB)
	assert.NotErrorIs(t, err, utils.ErrEventDestinationNotFound, "an outage is not reported as not-found")

	repo.GetDestinationFunc = func(context.Context, string, uuid.UUID) (*models.EventDestination, error) {
		return &models.EventDestination{}, nil
	}
	repo.SubscriptionsUsingFunc = func(context.Context, string, uuid.UUID) ([]models.EventSubscription, error) {
		return nil, errDB
	}
	_, err = svc.Usage(ctx, "ou", uuid.New())
	require.ErrorIs(t, err, errDB)

	repo.DestinationStatsOfFunc = func(context.Context, string, uuid.UUID) (repositories.DestinationStats, error) {
		return repositories.DestinationStats{}, errDB
	}
	_, err = svc.Stats(ctx, "ou", uuid.New())
	require.ErrorIs(t, err, errDB)

	repo.DestinationStatsFunc = func(context.Context, string) (map[uuid.UUID]repositories.DestinationStats, error) {
		return nil, errDB
	}
	_, _, err = svc.List(ctx, "ou")
	require.ErrorIs(t, err, errDB)
	repo.ListDestinationsFunc = func(context.Context, string) ([]models.EventDestination, error) { return nil, errDB }
	_, _, err = svc.Available(ctx, "ou", "p")
	require.ErrorIs(t, err, errDB)
}
