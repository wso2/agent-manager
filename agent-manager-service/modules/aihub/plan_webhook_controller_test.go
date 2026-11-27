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

package aihub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

const testSecret = "s3cret-shared-with-the-sender"

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"event_id":"e1"}`)
	now := time.Unix(1_800_000_000, 0)
	good := Sign(testSecret, body, now)

	assert.NoError(t, verifySignature(testSecret, good, body, 5*time.Minute, now))
	assert.NoError(t, verifySignature(testSecret, good, body, 5*time.Minute, now.Add(4*time.Minute)))
	assert.ErrorIs(t, verifySignature(testSecret, good, body, 5*time.Minute, now.Add(6*time.Minute)),
		errTimestampOutOfTolerance, "a stale signature is a possible replay")
	assert.ErrorIs(t, verifySignature(testSecret, good, body, 5*time.Minute, now.Add(-6*time.Minute)),
		errTimestampOutOfTolerance, "so is a future-dated one")
	assert.ErrorIs(t, verifySignature("another-secret", good, body, 5*time.Minute, now), errSignatureInvalid)
	assert.ErrorIs(t, verifySignature(testSecret, good, []byte(`{"event_id":"e2"}`), 5*time.Minute, now),
		errSignatureInvalid, "the signature covers the body")
	assert.ErrorIs(t, verifySignature(testSecret, "", body, 5*time.Minute, now), errSignatureMissing)
	assert.ErrorIs(t, verifySignature(testSecret, "t=abc,v1=00", body, 5*time.Minute, now), errSignatureInvalid)
	assert.ErrorIs(t, verifySignature(testSecret, "v1=00", body, 5*time.Minute, now), errSignatureInvalid)
}

func TestSecretHash(t *testing.T) {
	h := SecretHash(testSecret)
	assert.Len(t, h, 64, "hex SHA3-256")
	assert.Equal(t, h, SecretHash(testSecret))
	assert.NotEqual(t, h, SecretHash(testSecret+"x"))
	assert.Equal(t, strings.ToLower(h), h)
}

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	r := &webhookReceiver{cfg: webhookConfig{Secret: testSecret, SignatureTolerance: time.Minute}, now: func() time.Time { return now }}
	withHash := []byte(`{"event_id":"e1","secret_hash":"` + SecretHash(testSecret) + `"}`)
	signer, err := r.authenticate(ctx, Sign(testSecret, withHash, now), withHash)
	require.NoError(t, err)
	assert.Equal(t, webhookSigner{Secret: testSecret}, signer, "the shared secret is for no one organization")

	otherHash := []byte(`{"event_id":"e1","secret_hash":"` + SecretHash("other") + `"}`)
	_, err = r.authenticate(ctx, Sign(testSecret, otherHash, now), otherHash)
	assert.ErrorIs(t, err, errSignatureInvalid, "a delivery naming a secret this receiver doesn't hold is refused")

	noHash := []byte(`{"event_id":"e1"}`)
	_, err = r.authenticate(ctx, Sign(testSecret, noHash, now), noHash)
	assert.ErrorIs(t, err, errSignatureInvalid, "without secret_hash, only when legacy deliveries are allowed")
	r.cfg.LegacySharedSecret = true
	_, err = r.authenticate(ctx, Sign(testSecret, noHash, now), noHash)
	assert.NoError(t, err)
}

// stubSecrets holds organizations' webhook secrets in memory, as the receiver reads them.
type stubSecrets struct {
	signers []webhookSigner
	err     error
}

func (s stubSecrets) signerByHash(_ context.Context, hash string) (webhookSigner, bool, error) {
	if s.err != nil {
		return webhookSigner{}, false, s.err
	}
	for _, sg := range s.signers {
		if SecretHash(sg.Secret) == hash {
			return sg, true, nil
		}
	}
	return webhookSigner{}, false, nil
}

func (s stubSecrets) signersForRef(_ context.Context, ref string, limit int) ([]webhookSigner, error) {
	var out []webhookSigner
	for _, sg := range s.signers {
		if (sg.OrgRef == ref || sg.OrgID == ref) && len(out) < limit {
			out = append(out, sg)
		}
	}
	return out, s.err
}

// An organization's webhook secret signs only for that organization, named by its id or the reference the
// secret was given; the shared secret only for an organization named by its id.
func TestAuthenticateWithOrganizationSecrets(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	acme, globex := uuid.NewString(), uuid.NewString()
	acmePortal := webhookSigner{Secret: "whsec_acme", OrgID: acme, OrgRef: "default"}
	acmeSecondPortal := webhookSigner{Secret: "whsec_acme2", OrgID: acme}
	globexPortal := webhookSigner{Secret: "whsec_globex", OrgID: globex, OrgRef: "globex"}
	r := &webhookReceiver{
		cfg:     webhookConfig{SignatureTolerance: time.Minute},
		secrets: stubSecrets{signers: []webhookSigner{acmePortal, acmeSecondPortal, globexPortal}},
		core:    knownOrgs{ids: []string{acme, globex}},
		now:     func() time.Time { return now },
	}
	body := func(ref, secret string) []byte {
		return []byte(`{"event_id":"e1","org":{"ref_id":"` + ref + `"},"secret_hash":"` + SecretHash(secret) + `"}`)
	}

	t.Run("the secret a delivery names verifies it and fixes the organization", func(t *testing.T) {
		for _, tc := range []struct {
			signer webhookSigner
			ref    string
		}{{acmePortal, "default"}, {acmePortal, acme}, {acmeSecondPortal, acme}, {globexPortal, "globex"}} {
			b := body(tc.ref, tc.signer.Secret)
			signer, err := r.authenticate(ctx, Sign(tc.signer.Secret, b, now), b)
			require.NoError(t, err, tc.ref)
			assert.Equal(t, tc.signer, signer)
			org, err := r.orgOf(ctx, tc.ref, signer)
			require.NoError(t, err, tc.ref)
			assert.Equal(t, tc.signer.OrgID, org)
		}
	})

	t.Run("a secret of one organization doesn't sign for another", func(t *testing.T) {
		for _, ref := range []string{globex, "globex", "other"} {
			_, err := r.orgOf(ctx, ref, acmePortal)
			assert.ErrorIs(t, err, errOrganizationNotFound, ref)
		}
		_, err := r.orgOf(ctx, "default", acmeSecondPortal)
		assert.ErrorIs(t, err, errOrganizationNotFound, "a secret with no reference takes the organization id only")
	})

	t.Run("a delivery signed with another secret than it names is refused", func(t *testing.T) {
		b := body("default", acmePortal.Secret)
		_, err := r.authenticate(ctx, Sign(globexPortal.Secret, b, now), b)
		assert.ErrorIs(t, err, errSignatureInvalid)
	})

	t.Run("no shared secret: an empty one never verifies", func(t *testing.T) {
		b := body(acme, "")
		_, err := r.authenticate(ctx, Sign("", b, now), b)
		assert.ErrorIs(t, err, errSignatureInvalid, "anyone can sign with an empty secret")
	})

	t.Run("legacy: no secret_hash, tried against the secrets of the organization it names", func(t *testing.T) {
		r.cfg.LegacySharedSecret = true
		defer func() { r.cfg.LegacySharedSecret = false }()
		b := []byte(`{"event_id":"e1","org":{"ref_id":"globex"}}`)
		signer, err := r.authenticate(ctx, Sign(globexPortal.Secret, b, now), b)
		require.NoError(t, err)
		assert.Equal(t, globexPortal, signer)
		_, err = r.authenticate(ctx, Sign(acmePortal.Secret, b, now), b)
		assert.ErrorIs(t, err, errSignatureInvalid, "only that organization's secrets are tried")
		_, err = r.authenticate(ctx, Sign(globexPortal.Secret, b, now.Add(-time.Hour)), b)
		assert.ErrorIs(t, err, errTimestampOutOfTolerance)
	})

	t.Run("a failure reading the secrets is ours, not the sender's", func(t *testing.T) {
		failing := &webhookReceiver{cfg: r.cfg, secrets: stubSecrets{err: errors.New("database is down")}, now: r.now}
		b := body(acme, acmePortal.Secret)
		_, err := failing.authenticate(ctx, Sign(acmePortal.Secret, b, now), b)
		require.Error(t, err)
		assert.False(t, isClientFault(err))
	})
}

func TestWebhookSecretValue(t *testing.T) {
	v, err := newWebhookSecretValue()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(v, "whsec_"))
	assert.Len(t, v, len("whsec_")+64)
	other, err := newWebhookSecretValue()
	require.NoError(t, err)
	assert.NotEqual(t, v, other)
	assert.Equal(t, "whsec_****"+v[len(v)-4:], maskWebhookSecret(v))
	assert.Equal(t, "whsec_****", maskWebhookSecret("short"))
	assert.Equal(t, "webhook-secrets/abc/v2", webhookSecretPath("abc", 2))
}

func TestWebhookSecretInput(t *testing.T) {
	in := webhookSecretInput{Handle: " acme-portal ", OrgRef: " default "}
	require.NoError(t, in.normalize())
	assert.Equal(t, webhookSecretInput{Handle: "acme-portal", DisplayName: "acme-portal", OrgRef: "default"}, in)
	for name, bad := range map[string]webhookSecretInput{
		"no id":      {},
		"bad id":     {Handle: "Acme Portal"},
		"long id":    {Handle: strings.Repeat("a", maxHandleChars+1)},
		"long name":  {Handle: "a", DisplayName: strings.Repeat("n", maxWebhookSecretNameChars+1)},
		"long ref":   {Handle: "a", OrgRef: strings.Repeat("r", maxWebhookOrgRefChars+1)},
		"hyphen end": {Handle: "acme-"},
	} {
		var v *validationError
		assert.ErrorAs(t, bad.normalize(), &v, name)
	}
}

func TestEnvelope(t *testing.T) {
	env, err := decodeEnvelope([]byte(`{"event_id":"e1","event_type":"subscription_plan.created","org":{"ref_id":"acme"},"occurred_at":"2026-11-18T10:00:00Z"}`))
	require.NoError(t, err)
	require.NoError(t, env.validate())
	assert.Equal(t, "acme", env.OrgID, "org.ref_id is the organization handle")
	assert.Equal(t, time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC), env.occurredAt())

	legacy, err := decodeEnvelope([]byte(`{"event_id":"e1","event_type":"x","org_id":"acme"}`))
	require.NoError(t, err)
	assert.Equal(t, "acme", legacy.OrgID, "the flat org_id is still read")

	for name, body := range map[string]string{
		"no event id":   `{"event_type":"x","org":{"ref_id":"acme"}}`,
		"no event type": `{"event_id":"e1","org":{"ref_id":"acme"}}`,
		"no org":        `{"event_id":"e1","event_type":"x"}`,
	} {
		env, err := decodeEnvelope([]byte(body))
		require.NoError(t, err, name)
		assert.ErrorIs(t, env.validate(), errInvalidEnvelope, name)
	}
	_, err = decodeEnvelope([]byte(`not json`))
	assert.ErrorIs(t, err, errInvalidEnvelope)
}

func TestRequestWindowsFrom(t *testing.T) {
	got := requestWindowsFrom([]webhookLimit{
		{LimitType: "REQUEST_COUNT", LimitCount: 100, TimeUnit: "minute", TimeAmount: 0},
		{LimitType: "REQUEST_COUNT", LimitCount: -1, TimeUnit: "HOUR"},                 // unlimited
		{LimitType: "BANDWIDTH", LimitCount: 5, TimeUnit: "HOUR"},                      // unsupported
		{LimitType: "REQUEST_COUNT", LimitCount: 7, TimeUnit: "WEEK"},                  // unknown unit
		{LimitType: "REQUEST_COUNT", LimitCount: 9, TimeUnit: "MINUTE", TimeAmount: 1}, // same window length
		{LimitType: "request_count", LimitCount: 1000, TimeUnit: "DAY", TimeAmount: 1},
	})
	assert.Equal(t, []Window{{Count: 100, TimeAmount: 1, TimeUnit: "MINUTE"}, {Count: 1000, TimeAmount: 1, TimeUnit: "DAY"}}, got)
}

func TestWebhookConditions(t *testing.T) {
	allow := Condition{
		ID: ConditionIDRequests, Type: ConditionTypeRequestCount,
		Params: json.RawMessage(`{"limits":[{"count":5,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"allow"}`),
	}
	custom := Condition{ID: "region", Type: ConditionTypeCustom, Params: json.RawMessage(`{"a":1}`)}
	current, err := normalizeConditions([]Condition{allow, custom})
	require.NoError(t, err)

	t.Run("conditions in the event replace all", func(t *testing.T) {
		got, err := webhookConditions(current, &[]Condition{custom}, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "region", got[0].ID)
	})
	t.Run("limits from an older sender replace only the request-count condition, keeping onExceed", func(t *testing.T) {
		got, err := webhookConditions(current, nil, []Window{{Count: 50, TimeAmount: 1, TimeUnit: "HOUR"}})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.JSONEq(t, `{"limits":[{"count":50,"timeAmount":1,"timeUnit":"HOUR"}],"onExceed":"allow"}`, string(got[0].Params))
		assert.Equal(t, "region", got[1].ID)
	})
	t.Run("no limits remove the request-count condition", func(t *testing.T) {
		got, err := webhookConditions(current, nil, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "region", got[0].ID)
	})
}

func TestWebhookConfigFrom(t *testing.T) {
	settings := map[string]string{}
	get := func(k string) string { return settings[k] }
	cfg, err := webhookConfigFrom(get)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled, "on without a shared secret: organizations' webhook secrets still sign")
	assert.False(t, cfg.LegacySharedSecret)
	assert.Empty(t, cfg.Secret)
	assert.Equal(t, DefaultWebhookSignatureHeader, cfg.SignatureHeader)
	assert.Equal(t, 5*time.Minute, cfg.SignatureTolerance)
	assert.EqualValues(t, 1<<20, cfg.MaxBodyBytes)
	assert.Equal(t, DefaultWebhookSecretMaxVersions, cfg.SecretMaxVersions)

	settings["webhook_secret"] = testSecret
	settings["webhook_signature_tolerance"] = "90s"
	settings["webhook_max_body_size"] = "2048"
	settings["webhook_secret_max_versions"] = "5"
	settings["webhook_legacy_shared_secret"] = "true"
	cfg, err = webhookConfigFrom(get)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.LegacySharedSecret)
	assert.Equal(t, testSecret, cfg.Secret)
	assert.Equal(t, 90*time.Second, cfg.SignatureTolerance)
	assert.EqualValues(t, 2048, cfg.MaxBodyBytes)
	assert.Equal(t, 5, cfg.SecretMaxVersions)

	settings["webhook_enabled"] = "false"
	cfg, err = webhookConfigFrom(get)
	require.NoError(t, err)
	assert.False(t, cfg.Enabled, "and can be switched off")

	// A setting that is set but invalid stops the module from activating, rather than leaving a default in place.
	for key, value := range map[string]string{
		"webhook_enabled":              "flase",
		"webhook_legacy_shared_secret": "yes please",
		"webhook_secret_max_versions":  "0",
		"webhook_signature_tolerance":  "-1m",
		"webhook_max_body_size":        "lots",
	} {
		bad := map[string]string{key: value}
		_, err := webhookConfigFrom(func(k string) string { return bad[k] })
		require.Error(t, err, key)
		assert.Contains(t, err.Error(), key)
	}
}

func TestReceiverRejectsAlike(t *testing.T) {
	r := &webhookReceiver{cfg: webhookConfig{
		Enabled: true, Secret: testSecret, SignatureHeader: DefaultWebhookSignatureHeader,
		SignatureTolerance: time.Minute, MaxBodyBytes: 1 << 20,
	}, log: slog.New(slog.DiscardHandler), now: time.Now}
	for name, header := range map[string]string{"no signature": "", "bad signature": "t=1,v1=00"} {
		req := httptest.NewRequest(http.MethodPost, WebhookRoutePath, strings.NewReader(`{"event_id":"e1"}`))
		if header != "" {
			req.Header.Set(DefaultWebhookSignatureHeader, header)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.JSONEq(t, `{"status":"error","code":"UNAUTHORIZED","message":"Invalid or expired credentials."}`, rec.Body.String(), name)
	}
	assert.True(t, isClientFault(errSignatureInvalid))
	assert.False(t, isClientFault(errors.New("database is down")))
}

// knownOrgs answers organization lookups for the ids it holds.
type knownOrgs struct {
	modkit.CoreAPI
	ids []string
}

func (k knownOrgs) GetOrg(_ context.Context, id string) (modkit.Org, error) {
	for _, known := range k.ids {
		if known == id {
			return modkit.Org{ID: id, Handle: id}, nil
		}
	}
	return modkit.Org{}, modkit.ErrNotFound
}

// failingCore fails every organization lookup, as a database that is down would.
type failingCore struct{ modkit.CoreAPI }

func (failingCore) GetOrg(context.Context, string) (modkit.Org, error) {
	return modkit.Org{}, errors.New("database is down")
}

// A failure on this side answers 503, not 401, which would blame the sender for it.
func TestReceiverAnswersOurFailuresWithServiceUnavailable(t *testing.T) {
	r := &webhookReceiver{cfg: webhookConfig{
		Enabled: true, Secret: testSecret, SignatureHeader: DefaultWebhookSignatureHeader,
		SignatureTolerance: time.Minute, MaxBodyBytes: 1 << 20,
	}, core: failingCore{}, log: slog.New(slog.DiscardHandler), now: time.Now}
	body := []byte(`{"event_id":"e1","event_type":"subscription_plan.deleted","org":{"ref_id":"` + uuid.NewString() + `"},` +
		`"secret_hash":"` + SecretHash(testSecret) + `","data":{"handle":"gold"}}`)
	req := httptest.NewRequest(http.MethodPost, WebhookRoutePath, bytes.NewReader(body))
	req.Header.Set(DefaultWebhookSignatureHeader, Sign(testSecret, body, time.Now()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"error","code":"SERVICE_UNAVAILABLE","message":"The delivery could not be processed. Retry later."}`,
		rec.Body.String())
	assert.True(t, isClientFault(errWebhookDisabled), "a disabled webhook stays a 401: a retry can't succeed")
}

// recordedAudit keeps what a request adds to its audit record.
type recordedAudit struct {
	resource []string
	details  map[string]any
}

func (a *recordedAudit) AuditDetail(key string, value any) { a.details[key] = value }
func (a *recordedAudit) AuditResource(kind, id, name string) {
	a.resource = []string{kind, id, name}
}
func (a *recordedAudit) AuditEnvironment(string)                                    {}
func (a *recordedAudit) AuditInternalCaller(context.Context, modkit.InternalCaller) {}

// A webhook secret change names the secret by its handle and the version, and nothing of the value.
func TestWebhookSecretAuditRecord(t *testing.T) {
	rec := &recordedAudit{details: map[string]any{}}
	ctx := modkit.WithAuditScope(context.Background(), rec)
	secret := &webhookSecret{
		Handle: "acme-portal", DisplayName: "Acme portal",
		Versions: []webhookSecretVersion{{Version: 3}, {Version: 1}, {Version: 2}},
	}

	auditSecretVersion(ctx, secret.Handle, secret.DisplayName, newestVersion(secret))
	assert.Equal(t, []string{auditResourceWebhookSecret, "acme-portal", "Acme portal"}, rec.resource)
	assert.Equal(t, map[string]any{auditDetailVersion: 3}, rec.details)

	rec.details = map[string]any{}
	auditSecretVersion(ctx, "acme-portal", "acme-portal", 0)
	assert.Empty(t, rec.details, "no version, no detail")

	declared := map[string]bool{}
	for _, a := range Module().Manifest().AuditActions {
		if a.Class == "credential" {
			declared[a.Name] = a.Details[auditDetailVersion] == "count"
		}
	}
	assert.Equal(t, map[string]bool{
		PermWebhookSecretCreate: true, PermWebhookSecretRotate: true, PermWebhookSecretDelete: true,
	}, declared, "each change is a credential action that declares its version")
}
