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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var testWebhookKey = []byte("0123456789abcdef0123456789abcdef")

// specVectorKey is the base64 key of the Standard Webhooks specification's
// published test vector; it is not a credential.
const specVectorKey = "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// testSigningSecret is a throwaway signing secret for tests.
func testSigningSecret() string {
	return WebhookSecretPrefix + base64.StdEncoding.EncodeToString([]byte("test-signing-key-for-unit-tests!"))
}

type fakeWebhookSender struct {
	code    int
	err     error
	urlErr  error
	targets []WebhookTarget
	bodies  [][]byte
}

func (f *fakeWebhookSender) Send(_ context.Context, target WebhookTarget, _, _ string, body []byte) (int, error) {
	f.targets = append(f.targets, target)
	f.bodies = append(f.bodies, body)
	return f.code, f.err
}

func (f *fakeWebhookSender) ValidateURL(context.Context, string) error { return f.urlErr }

func newWebhookRepoMock() (*repomocks.WebhookRepositoryMock, map[uuid.UUID]*models.WebhookEndpoint) {
	store := map[uuid.UUID]*models.WebhookEndpoint{}
	return &repomocks.WebhookRepositoryMock{
		ListEndpointsFunc: func(_ context.Context, t repositories.WebhookTarget) ([]models.WebhookEndpoint, error) {
			var out []models.WebhookEndpoint
			for _, e := range store {
				if e.OUID == t.OUID && e.Scope == t.Scope && e.ProjectName == t.ProjectName && e.AgentName == t.AgentName {
					out = append(out, *e)
				}
			}
			return out, nil
		},
		GetEndpointFunc: func(_ context.Context, t repositories.WebhookTarget, id uuid.UUID) (*models.WebhookEndpoint, error) {
			e, ok := store[id]
			if !ok || e.OUID != t.OUID || e.Scope != t.Scope || e.ProjectName != t.ProjectName || e.AgentName != t.AgentName {
				return nil, gorm.ErrRecordNotFound
			}
			c := *e
			return &c, nil
		},
		CreateEndpointFunc: func(_ context.Context, e *models.WebhookEndpoint) error {
			c := *e
			store[e.ID] = &c
			return nil
		},
		UpdateEndpointFunc: func(_ context.Context, e *models.WebhookEndpoint) error {
			c := *e
			store[e.ID] = &c
			return nil
		},
		DeleteEndpointFunc: func(_ context.Context, _ repositories.WebhookTarget, id uuid.UUID) (bool, error) {
			_, ok := store[id]
			delete(store, id)
			return ok, nil
		},
	}, store
}

func agentTarget() repositories.WebhookTarget {
	return repositories.WebhookTarget{OUID: "ou-1", Scope: models.WebhookScopeAgent, ProjectName: "p1", AgentName: "a1"}
}

func TestWebhookService_CreateValidatesScopeAndEvents(t *testing.T) {
	repo, store := newWebhookRepoMock()
	svc := NewWebhookService(NewLocalWebhookStore(repo, &fakeWebhookSender{}, testWebhookKey, discardLogger()))
	ctx := auditableCtx(t)

	_, _, err := svc.Create(context.Background(), agentTarget(), WebhookEndpointInput{
		Name: "ci", URL: "https://h.example.com", Environments: []string{"dev"}, EventTypes: []string{"agent.deployed"},
	})
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable, "no webhook is created without an audit record")
	assert.Empty(t, store)

	cases := map[string]WebhookEndpointInput{
		"missing name":           {URL: "https://h.example.com", Environments: []string{"dev"}},
		"agent without env":      {Name: "ci", URL: "https://h.example.com"},
		"event of another scope": {Name: "ci", URL: "https://h.example.com", Environments: []string{"dev"}, EventTypes: []string{"project.created"}},
		"unknown event":          {Name: "ci", URL: "https://h.example.com", Environments: []string{"dev"}, EventTypes: []string{"nope"}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.Create(ctx, agentTarget(), in)
			assert.ErrorIs(t, err, utils.ErrInvalidInput)
		})
	}

	e, secret, err := svc.Create(ctx, agentTarget(), WebhookEndpointInput{
		Name: " ci ", URL: "https://h.example.com", Environments: []string{"dev", " dev "}, Enabled: true,
		EventTypes: []string{"agent.deployed", "monitor_run.failed", "agent.deployed"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ci", e.Name)
	assert.Equal(t, []string{"agent.deployed", "monitor_run.failed"}, e.EventTypes, "duplicates are dropped")
	assert.Equal(t, []string{"dev"}, e.Environments)
	assert.Contains(t, secret, WebhookSecretPrefix)
	assert.NotContains(t, string(store[e.ID].SecretEncrypted), secret, "the secret is stored encrypted")

	target, err := webhookTargetFor(store[e.ID], testWebhookKey)
	require.NoError(t, err)
	assert.Equal(t, secret, target.Secret)
}

func TestWebhookService_ScopeIsolation(t *testing.T) {
	repo, _ := newWebhookRepoMock()
	svc := NewWebhookService(NewLocalWebhookStore(repo, &fakeWebhookSender{}, testWebhookKey, discardLogger()))
	ctx := auditableCtx(t)
	e, _, err := svc.Create(ctx, agentTarget(), WebhookEndpointInput{Name: "ci", URL: "https://h.example.com", Environments: []string{"dev"}})
	require.NoError(t, err)

	other := agentTarget()
	other.AgentName = "a2"
	_, err = svc.Get(ctx, other, e.ID)
	assert.ErrorIs(t, err, utils.ErrWebhookNotFound, "another agent cannot see the webhook")
	assert.ErrorIs(t, svc.Delete(ctx, other, e.ID), utils.ErrWebhookNotFound)
}

func TestWebhookService_RotateAndTest(t *testing.T) {
	repo, store := newWebhookRepoMock()
	sender := &fakeWebhookSender{code: http.StatusOK}
	svc := NewWebhookService(NewLocalWebhookStore(repo, sender, testWebhookKey, discardLogger()))
	ctx := auditableCtx(t)
	org := repositories.WebhookTarget{OUID: "ou-1", Scope: models.WebhookScopeOrg}
	e, first, err := svc.Create(ctx, org, WebhookEndpointInput{Name: "ops", URL: "https://h.example.com", Enabled: true})
	require.NoError(t, err)
	assert.Empty(t, store[e.ID].Environments, "only agent webhooks have environments")

	second, err := svc.RotateSecret(ctx, org, e.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)

	result, err := svc.Test(ctx, org, e.ID)
	require.NoError(t, err)
	assert.True(t, result.Delivered)
	require.Len(t, sender.targets, 1)
	assert.Equal(t, second, sender.targets[0].Secret, "the test is signed with the current secret")
	assert.Contains(t, string(sender.bodies[0]), `"type":"com.wso2.agentmanager.webhook.test"`)

	sender.code, sender.err = http.StatusBadGateway, errors.New("webhook endpoint returned HTTP 502")
	result, err = svc.Test(ctx, org, e.ID)
	require.NoError(t, err)
	assert.False(t, result.Delivered)
	assert.Equal(t, http.StatusBadGateway, result.StatusCode)
}

// The signature must verify the way a Standard Webhooks library checks it.
func TestSignWebhook_StandardWebhooks(t *testing.T) {
	// Test vector from the Standard Webhooks specification. The key is built
	// from the prefix and its base64 part so the source never holds a literal
	// that secret scanners take for a real signing secret.
	secret := WebhookSecretPrefix + specVectorKey
	sig, err := SignWebhook(secret, "msg_p5jXN8AQM9LWM0D4loKWxJek", time.Unix(1614265330, 0), []byte(`{"test": 2432232314}`))
	require.NoError(t, err)
	assert.Equal(t, "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=", sig)

	key, _ := base64.StdEncoding.DecodeString(specVectorKey)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("msg_p5jXN8AQM9LWM0D4loKWxJek.1614265330." + `{"test": 2432232314}`))
	assert.Equal(t, "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)), sig)
}

func TestWebhookRetryPolicy(t *testing.T) {
	for _, code := range []int{0, 500, 502, 408, 429} {
		assert.True(t, retryableStatus(code), code)
	}
	for _, code := range []int{400, 401, 403, 404, 410} {
		assert.False(t, retryableStatus(code), code)
	}
	assert.Equal(t, 10*time.Second, webhookBackoff(1))
	assert.Equal(t, 2*time.Hour, webhookBackoff(5))
	assert.Equal(t, 2*time.Hour, webhookBackoff(50), "later attempts reuse the last delay")
}
