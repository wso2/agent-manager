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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The "api" instance's remote store talks to a dispatcher's internal API:
// every call round-trips, errors map back to the local store's, and the
// encrypted secret never leaves the dispatcher.
func TestRemoteWebhookStore_RoundTripsThroughInternalAPI(t *testing.T) {
	repo, stored := newWebhookRepoMock()
	removed := ""
	repo.DeleteEndpointsUnderFunc = func(_ context.Context, ouID, project, agent string) error {
		removed = ouID + "/" + project + "/" + agent
		return nil
	}
	sender := &fakeWebhookSender{code: http.StatusOK}
	local := NewLocalWebhookStore(repo, sender, testWebhookKey, discardLogger())

	var lastBody string
	api := NewWebhookInternalAPI(local, "shared-key", discardLogger())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.ServeHTTP(&bodySpy{ResponseWriter: w, body: &lastBody}, r)
	}))
	defer server.Close()

	remote := NewRemoteWebhookStore(server.URL, "shared-key", discardLogger())
	ctx := context.Background()
	target := repositories.WebhookTarget{OUID: "ou-1", Scope: models.WebhookScopeAgent, ProjectName: "p1", AgentName: "a1"}

	e, secret, err := remote.Create(ctx, target, WebhookEndpointInput{
		Name: "ci", URL: "https://h.example.com/hook", Environments: []string{"dev"},
		EventTypes: []string{"agent.deployed"}, Enabled: true, CreatedBy: "user-1",
	})
	require.NoError(t, err)
	assert.Contains(t, secret, WebhookSecretPrefix)
	assert.Equal(t, []string{"dev"}, e.Environments)
	assert.Equal(t, "user-1", stored[e.ID].CreatedBy, "the creator travels with the request")
	assert.NotContains(t, lastBody, "ecret_encrypted")
	assert.NotContains(t, lastBody, "SecretEncrypted", "the encrypted secret is never serialized")

	list, err := remote.List(ctx, target)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Nil(t, list[0].SecretEncrypted)

	_, err = remote.Get(ctx, target, uuid.New())
	assert.ErrorIs(t, err, utils.ErrWebhookNotFound)

	_, _, err = remote.Create(ctx, target, WebhookEndpointInput{Name: "x", URL: "https://h.example.com"})
	require.ErrorIs(t, err, utils.ErrInvalidInput, "validation errors map back to invalid input")
	assert.Equal(t, 1, strings.Count(err.Error(), "invalid input"), "the prefix is not repeated")

	result, err := remote.Test(ctx, target, e.ID)
	require.NoError(t, err)
	assert.True(t, result.Delivered)

	rotated, err := remote.RotateSecret(ctx, target, e.ID)
	require.NoError(t, err)
	assert.NotEqual(t, secret, rotated)

	remote.RemoveForAgent(ctx, "ou-1", "p1", "a1")
	assert.Equal(t, "ou-1/p1/a1", removed)

	require.NoError(t, remote.Delete(ctx, target, e.ID))
	assert.Empty(t, stored)

	wrongKey := NewRemoteWebhookStore(server.URL, "other-key", discardLogger())
	_, err = wrongKey.List(ctx, target)
	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrInvalidInput)
}

// bodySpy keeps the last response body.
type bodySpy struct {
	http.ResponseWriter
	body *string
}

func (b *bodySpy) Write(p []byte) (int, error) {
	*b.body = string(p)
	return b.ResponseWriter.Write(p)
}

var _ io.Writer = (*bodySpy)(nil)
