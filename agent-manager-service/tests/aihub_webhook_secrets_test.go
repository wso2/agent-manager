//go:build integration

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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aihub"
)

const webhookSecretsPath = "/m/ai-hub/webhook-secrets"

type webhookSecretVersionBody struct {
	Version      int    `json:"version"`
	MaskedSecret string `json:"maskedSecret"`
	Hash         string `json:"hash"`
	CreatedBy    string `json:"createdBy"`
}

type webhookSecretBody struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	OrgRef    string                     `json:"orgRef"`
	CreatedBy string                     `json:"createdBy"`
	Versions  []webhookSecretVersionBody `json:"versions"`
	Secret    string                     `json:"secret"`
}

// signedDelivery is a plan delivery signed with secret, naming the organization by ref, as a portal sends it.
func signedDelivery(t *testing.T, secret, ref, eventType string, data map[string]any) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"event_id": uuid.NewString(), "event_type": eventType, "occurred_at": time.Now().UTC().Format(time.RFC3339Nano),
		"org": map[string]any{"ref_id": ref}, "aggregate_type": "subscription_plan",
		"secret_hash": aihub.SecretHash(secret), "encrypted_fields": []string{}, "data": data,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, aihub.WebhookRoutePath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(aihub.DefaultWebhookSignatureHeader, aihub.Sign(secret, body, time.Now()))
	return req
}

// legacyDelivery is a plan delivery signed with secret, from a portal that predates secret_hash: it names no secret.
func legacyDelivery(t *testing.T, secret, ref, handle string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"event_id": uuid.NewString(), "event_type": "subscription_plan.created",
		"occurred_at": time.Now().UTC().Format(time.RFC3339Nano), "org": map[string]any{"ref_id": ref},
		"aggregate_type": "subscription_plan", "encrypted_fields": []string{}, "data": portalPlan(handle),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, aihub.WebhookRoutePath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(aihub.DefaultWebhookSignatureHeader, aihub.Sign(secret, body, time.Now()))
	return req
}

func portalPlan(handle string) map[string]any {
	return map[string]any{"handle": handle, "display_name": handle, "pricing": nil}
}

// TestWebhookSecrets drives the organizations' webhook secrets through the module's API, and the deliveries they
// sign through the receiver, with no shared webhook_secret set: several portals sending to one organization,
// two organizations whose portals send the same reference, rotation, and deletion.
func TestWebhookSecrets(t *testing.T) {
	t.Setenv("MODULE_AI_HUB_WEBHOOK_SECRET", "")
	w := newPlansWorld(t)
	ctx := context.Background()
	deactivatePlans(ctx, t, w)
	installPlans(ctx, t, w)
	suffix := uuid.NewString()[:6]
	acmePortal, acmeSecond, globexPortal := "acme-portal-"+suffix, "acme-second-"+suffix, "globex-portal-"+suffix

	create := func(h http.Handler, id, orgRef string) webhookSecretBody {
		t.Helper()
		code, body := w.call(t, h, http.MethodPost, webhookSecretsPath, map[string]any{"id": id, "name": "Portal " + id, "orgRef": orgRef})
		require.Equal(t, http.StatusCreated, code, string(body))
		return decodeAs[webhookSecretBody](t, body)
	}
	deliver := func(secret, ref, handle string) (int, string) {
		t.Helper()
		return send(t, w.internal, signedDelivery(t, secret, ref, "subscription_plan.created", portalPlan(handle)))
	}
	planIn := func(h http.Handler, handle string) bool {
		t.Helper()
		code, _ := w.call(t, h, http.MethodGet, plansPath+"/"+handle, nil)
		return code == http.StatusOK
	}

	var acme, second, globex webhookSecretBody
	t.Run("create: the value is returned once, then only masked, with the hash deliveries name", func(t *testing.T) {
		code, body := w.call(t, w.limited, http.MethodPost, webhookSecretsPath, map[string]any{"id": acmePortal})
		assert.Equal(t, http.StatusForbidden, code, "creating one needs its permission: "+string(body))

		acme = create(w.full, acmePortal, "default")
		require.True(t, strings.HasPrefix(acme.Secret, "whsec_"), acme.Secret)
		assert.Len(t, acme.Secret, len("whsec_")+64)
		assert.Equal(t, "default", acme.OrgRef)
		require.Len(t, acme.Versions, 1)
		assert.Equal(t, 1, acme.Versions[0].Version)
		assert.Equal(t, aihub.SecretHash(acme.Secret), acme.Versions[0].Hash)
		assert.Equal(t, "module-tester", acme.CreatedBy)

		code, body = w.call(t, w.full, http.MethodGet, webhookSecretsPath+"/"+acmePortal, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.NotContains(t, string(body), acme.Secret, "a read never returns the value")
		assert.NotContains(t, string(body), `"secret"`)
		got := decodeAs[webhookSecretBody](t, body)
		assert.Equal(t, "whsec_****"+acme.Secret[len(acme.Secret)-4:], got.Versions[0].MaskedSecret)

		code, body = w.call(t, w.full, http.MethodPost, webhookSecretsPath, map[string]any{"id": acmePortal})
		assert.Equal(t, http.StatusConflict, code, string(body))
		assert.Contains(t, string(body), "WEBHOOK_SECRET_EXISTS")
		code, body = w.call(t, w.full, http.MethodPost, webhookSecretsPath, map[string]any{"id": "Not A Handle"})
		assert.Equal(t, http.StatusBadRequest, code, string(body))

		second = create(w.full, acmeSecond, "")
		// Another organization's portal sends the same reference: it is that organization's secret.
		globex = create(w.other, globexPortal, "default")
	})

	t.Run("several portals send to one organization; a portal reference maps to it", func(t *testing.T) {
		code, body := deliver(acme.Secret, "default", "by-ref-"+suffix)
		require.Equal(t, http.StatusOK, code, body)
		assert.True(t, planIn(w.full, "by-ref-"+suffix), "the secret's organization got it")
		assert.False(t, planIn(w.other, "by-ref-"+suffix), "not the other organization with the same reference")

		code, body = deliver(acme.Secret, w.org, "by-id-"+suffix)
		require.Equal(t, http.StatusOK, code, "the organization id is accepted too: "+body)

		code, body = deliver(second.Secret, w.org, "second-"+suffix)
		require.Equal(t, http.StatusOK, code, body)
		assert.True(t, planIn(w.full, "second-"+suffix))

		code, body = deliver(globex.Secret, "default", "globex-"+suffix)
		require.Equal(t, http.StatusOK, code, body)
		assert.True(t, planIn(w.other, "globex-"+suffix))
		assert.False(t, planIn(w.full, "globex-"+suffix))
	})

	t.Run("a secret signs only for its organization", func(t *testing.T) {
		for name, req := range map[string]*http.Request{
			"another organization's id":           signedDelivery(t, acme.Secret, w.otherOrg, "subscription_plan.created", portalPlan("x-"+suffix)),
			"a reference the secret wasn't given": signedDelivery(t, second.Secret, "default", "subscription_plan.created", portalPlan("x-"+suffix)),
			"a handle":                            signedDelivery(t, acme.Secret, w.handle, "subscription_plan.created", portalPlan("x-"+suffix)),
			"an unknown secret":                   signedDelivery(t, "whsec_unknown", w.org, "subscription_plan.created", portalPlan("x-"+suffix)),
			"an empty secret":                     signedDelivery(t, "", w.org, "subscription_plan.created", portalPlan("x-"+suffix)),
		} {
			code, body := send(t, w.internal, req)
			assert.Equal(t, http.StatusUnauthorized, code, name+": "+body)
		}
		assert.False(t, planIn(w.full, "x-"+suffix))
		assert.False(t, planIn(w.other, "x-"+suffix))
	})

	var v2, v3 string
	t.Run("rotation: the old version signs until it is deleted; versions are capped", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, webhookSecretsPath+"/"+acmePortal+"/rotate", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		rotated := decodeAs[webhookSecretBody](t, body)
		v2 = rotated.Secret
		require.NotEqual(t, acme.Secret, v2)
		require.Len(t, rotated.Versions, 2)
		assert.Equal(t, 2, rotated.Versions[0].Version, "newest first")

		for _, secret := range []string{acme.Secret, v2} {
			code, body := deliver(secret, "default", "rot-"+uuid.NewString()[:6])
			assert.Equal(t, http.StatusOK, code, body)
		}
		code, body = w.call(t, w.full, http.MethodPost, webhookSecretsPath+"/"+acmePortal+"/rotate", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		v3 = decodeAs[webhookSecretBody](t, body).Secret
		code, body = w.call(t, w.full, http.MethodPost, webhookSecretsPath+"/"+acmePortal+"/rotate", nil)
		assert.Equal(t, http.StatusConflict, code, string(body))
		assert.Contains(t, string(body), "WEBHOOK_SECRET_VERSION_LIMIT")

		code, body = w.call(t, w.full, http.MethodDelete, webhookSecretsPath+"/"+acmePortal+"/versions/1", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		code, got := deliver(acme.Secret, "default", "old-"+suffix)
		assert.Equal(t, http.StatusUnauthorized, code, "a deleted version signs nothing: "+got)
		code, got = deliver(v3, "default", "new-"+suffix)
		assert.Equal(t, http.StatusOK, code, got)

		code, body = w.call(t, w.full, http.MethodDelete, webhookSecretsPath+"/"+acmePortal+"/versions/1", nil)
		assert.Equal(t, http.StatusNotFound, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, webhookSecretsPath+"/"+acmeSecond+"/versions/1", nil)
		assert.Equal(t, http.StatusConflict, code, string(body))
		assert.Contains(t, string(body), "WEBHOOK_SECRET_LAST_VERSION")
	})

	t.Run("delete: every version stops signing; list and read follow", func(t *testing.T) {
		code, body := w.call(t, w.other, http.MethodDelete, webhookSecretsPath+"/"+acmePortal, nil)
		assert.Equal(t, http.StatusNotFound, code, "another organization can't reach it: "+string(body))

		code, body = w.call(t, w.full, http.MethodDelete, webhookSecretsPath+"/"+acmePortal, nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		for _, secret := range []string{v2, v3} {
			code, got := deliver(secret, "default", "gone-"+uuid.NewString()[:6])
			assert.Equal(t, http.StatusUnauthorized, code, got)
		}
		code, _ = w.call(t, w.full, http.MethodGet, webhookSecretsPath+"/"+acmePortal, nil)
		assert.Equal(t, http.StatusNotFound, code)

		code, body = w.call(t, w.full, http.MethodGet, webhookSecretsPath, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var list struct {
			List  []webhookSecretBody `json:"list"`
			Count int                 `json:"count"`
		}
		require.NoError(t, json.Unmarshal(body, &list))
		var ids []string
		for _, s := range list.List {
			ids = append(ids, s.ID)
			assert.Empty(t, s.Secret)
		}
		assert.Contains(t, ids, acmeSecond)
		assert.NotContains(t, ids, acmePortal)
		assert.NotContains(t, ids, globexPortal, "only the caller's organization")
	})
}

// TestWebhookSecretsLegacyAndBadRequests covers deliveries from portals that predate secret_hash, accepted with
// webhook_legacy_shared_secret on, and the requests the webhook secrets API refuses.
func TestWebhookSecretsLegacyAndBadRequests(t *testing.T) {
	t.Setenv("MODULE_AI_HUB_WEBHOOK_SECRET", "")
	t.Setenv("MODULE_AI_HUB_WEBHOOK_LEGACY_SHARED_SECRET", "true")
	w := newPlansWorld(t)
	ctx := context.Background()
	deactivatePlans(ctx, t, w)
	installPlans(ctx, t, w)
	suffix := uuid.NewString()[:6]
	ref := "legacy-ref-" + suffix

	create := func(id, orgRef string) webhookSecretBody {
		t.Helper()
		code, body := w.call(t, w.full, http.MethodPost, webhookSecretsPath, map[string]any{"id": id, "name": id, "orgRef": orgRef})
		require.Equal(t, http.StatusCreated, code, string(body))
		return decodeAs[webhookSecretBody](t, body)
	}
	mapped := create("legacy-mapped-"+suffix, ref)
	plain := create("legacy-plain-"+suffix, "")

	t.Run("legacy deliveries are tried against the secrets of the organization they name", func(t *testing.T) {
		code, body := send(t, w.internal, legacyDelivery(t, mapped.Secret, ref, "legacy-by-ref-"+suffix))
		require.Equal(t, http.StatusOK, code, "by the secret's orgRef: "+body)
		code, body = send(t, w.internal, legacyDelivery(t, plain.Secret, w.org, "legacy-by-id-"+suffix))
		require.Equal(t, http.StatusOK, code, "by the organization's id: "+body)

		code, plan := w.call(t, w.full, http.MethodGet, plansPath+"/legacy-by-ref-"+suffix, nil)
		assert.Equal(t, http.StatusOK, code, string(plan))
		code, _ = w.call(t, w.other, http.MethodGet, plansPath+"/legacy-by-ref-"+suffix, nil)
		assert.Equal(t, http.StatusNotFound, code, "not the other organization")

		for name, req := range map[string]*http.Request{
			"another organization's id": legacyDelivery(t, plain.Secret, w.otherOrg, "legacy-x-"+suffix),
			"an unmapped reference":     legacyDelivery(t, plain.Secret, "unmapped-"+suffix, "legacy-x-"+suffix),
			"an unknown secret":         legacyDelivery(t, "whsec_unknown", w.org, "legacy-x-"+suffix),
		} {
			code, body := send(t, w.internal, req)
			assert.Equal(t, http.StatusUnauthorized, code, name+": "+body)
		}
	})

	t.Run("the API refuses bad requests", func(t *testing.T) {
		raw := func(method, path, body string) (int, string) {
			t.Helper()
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			return send(t, w.full, req)
		}
		code, body := raw(http.MethodPost, webhookSecretsPath, "{not json")
		assert.Equal(t, http.StatusBadRequest, code, body)
		code, body = raw(http.MethodPost, webhookSecretsPath, `{"id":"big","name":"`+strings.Repeat("a", 20<<10)+`"}`)
		assert.Equal(t, http.StatusRequestEntityTooLarge, code, body)

		for _, v := range []string{"abc", "0"} {
			code, body := w.call(t, w.full, http.MethodDelete, webhookSecretsPath+"/"+plain.ID+"/versions/"+v, nil)
			assert.Equal(t, http.StatusBadRequest, code, v+": "+string(body))
		}

		missing := webhookSecretsPath + "/missing-" + suffix
		for _, c := range []struct{ method, path string }{
			{http.MethodGet, missing},
			{http.MethodPost, missing + "/rotate"},
			{http.MethodDelete, missing},
			{http.MethodDelete, missing + "/versions/1"},
		} {
			code, body := w.call(t, w.full, c.method, c.path, nil)
			assert.Equal(t, http.StatusNotFound, code, c.method+" "+c.path+": "+string(body))
			assert.Contains(t, string(body), "WEBHOOK_SECRET_NOT_FOUND")
		}
	})
}
