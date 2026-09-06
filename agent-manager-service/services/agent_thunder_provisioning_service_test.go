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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type endpointResolverStub struct {
	managementURL string
}

func (r endpointResolverStub) ResolveEndpoints(context.Context, string, string, string, string, bool) (thundersvc.EnvThunderEndpoints, error) {
	return thundersvc.EnvThunderEndpoints{
		ManagementURL:    r.managementURL,
		TokenURL:         "https://token.example.com/oauth2/token",
		SystemTokenScope: "tenant_instance:system",
	}, nil
}

func TestAgentThunderEndpointResolverCanBeConfiguredAfterFactoryConstruction(t *testing.T) {
	holder := &agentThunderEndpointResolver{}
	service := &agentThunderProvisioningService{endpointResolver: holder}
	service.SetEnvThunderEndpointResolver(endpointResolverStub{managementURL: "http://internal.example.com"})

	endpoints, err := holder.ResolveEndpoints(context.Background(), "ou-id", "org", "dev-eu", "https://public.example.com", true)
	require.NoError(t, err)
	require.Equal(t, "http://internal.example.com", endpoints.ManagementURL)
}

// testOCClientResolvingSecretRefs returns an OpenChoreoClientMock whose
// GetSecretReference resolves ANY secret ref name to a "resolved/<name>" KV
// key holding the agent-identity client-secret data source — standing in for
// whatever path the real secret manager provider would have assigned when it
// created the SecretReference (see storeCredential: that path is never
// computed locally, only ever read back). Deliberately a different shape
// than agentIdentitySecretLocation(...).KVPath() would produce, so a test
// asserting a locally-guessed path would fail loudly instead of passing by
// coincidence.
func testOCClientResolvingSecretRefs() *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _ string, secretRefName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{
				Name: secretRefName,
				Data: []client.SecretDataSourceInfo{
					{
						SecretKey: thundersvc.AgentSecretKeyClientSecret,
						RemoteRef: client.RemoteRefInfo{Key: "resolved/" + secretRefName},
					},
				},
			}, nil
		},
	}
}

func newTestProvisioningService(
	repo *repomocks.AgentThunderClientRepositoryMock,
	resolver *clientmocks.EnvThunderResolverMock,
	store *clientmocks.SecretManagementClientMock,
) AgentThunderProvisioningService {
	return NewAgentThunderProvisioningService(repo, resolver, store, testOCClientResolvingSecretRefs(), nil, slog.Default())
}

// newTestProvisioningServiceWithInjector is newTestProvisioningService plus a
// workload injector, for tests asserting the post-provisioning Gateway Binding hook.
func newTestProvisioningServiceWithInjector(
	repo *repomocks.AgentThunderClientRepositoryMock,
	resolver *clientmocks.EnvThunderResolverMock,
	store *clientmocks.SecretManagementClientMock,
	injector AgentIdentityInjectionService,
) AgentThunderProvisioningService {
	return NewAgentThunderProvisioningService(repo, resolver, store, testOCClientResolvingSecretRefs(), injector, slog.Default())
}

func fakeThunderClientMock() *clientmocks.ThunderClientMock {
	return &clientmocks.ThunderClientMock{
		GetDefaultOUIDFunc: func(_ context.Context) (string, error) { return "ou-1", nil },
	}
}

func TestAttemptProvision_Success_CreatesIdentityAndStoresSecret(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, ouID, name, owner string) (string, string, string, bool, error) {
		assert.Equal(t, "ou-1", ouID)
		assert.Empty(t, owner, "owner must be left empty so Thunder defaults it to the caller's own subject")
		return "thunder-agent-1", "client-abc", "secret-xyz", true, nil
	}

	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			assert.Equal(t, "acme", location.OrgName)
			assert.Equal(t, "client-abc", data[thundersvc.AgentSecretKeyClientID])
			assert.Equal(t, "secret-xyz", data[thundersvc.AgentSecretKeyClientSecret])
			return "ref", nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}

	var recorded repositories.AgentThunderAttemptUpdate
	var recordedID uuid.UUID
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, id uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recordedID = id
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	bindingID := uuid.New()
	binding := models.AgentThunderClient{
		ID: bindingID, OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, bindingID, recordedID)
	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	require.NotNil(t, recorded.ThunderAgentID)
	assert.Equal(t, "thunder-agent-1", *recorded.ThunderAgentID)
	require.NotNil(t, recorded.ThunderClientID)
	assert.Equal(t, "client-abc", *recorded.ThunderClientID)
	require.NotNil(t, recorded.SecretRefPath)
	assert.Equal(t, "resolved/ref", *recorded.SecretRefPath,
		"the persisted SecretRefPath is resolved by reading the SecretReference back from OpenChoreo, never CreateSecret's bare name or a locally-computed path")
	assert.Empty(t, recorded.LastError)
}

// TestAttemptProvision_Success_ExternalAgent_NeverStoresSecret guards the
// earliest point a secret could leak into storage: even though Thunder
// returns one on identity creation, an external agent's provisioning attempt
// must discard it and complete with an empty SecretRefPath — RegenerateSecret
// is the only way to ever obtain one.
func TestAttemptProvision_Success_ExternalAgent_NeverStoresSecret(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "thunder-agent-1", "client-abc", "secret-xyz", true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("must not store a secret for an external agent")
			return "", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	require.NotNil(t, recorded.ThunderClientID)
	assert.Equal(t, "client-abc", *recorded.ThunderClientID, "the client_id is still recorded — only the secret is discarded")
	require.NotNil(t, recorded.SecretRefPath)
	assert.Empty(t, *recorded.SecretRefPath, "an external agent must complete provisioning with no stored secret ref")
}

func TestAttemptProvision_AlreadyHasThunderAgentID_SkipsCreate(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		t.Fatal("CreateAgentIdentity must not be called when the binding already has a thunderAgentID")
		return "", "", "", false, nil
	}
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		assert.Equal(t, "already-created", thunderAgentID)
		return "recovered-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var storedSecret string
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			storedSecret = data[thundersvc.AgentSecretKeyClientSecret]
			return "ref", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ThunderAgentID: "already-created", ThunderClientID: "already-client-id",
		// SecretRefPath deliberately empty: models a binding whose identity was
		// created on a prior attempt, but that attempt failed before a secret
		// was ever stored for it.
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	assert.Equal(t, "recovered-secret", storedSecret,
		"a binding with a Thunder identity but no stored secret must recover one before completing")
	require.NotNil(t, recorded.SecretRefPath,
		"the recovered secret's storage location must be persisted, not left empty")
	assert.Equal(t, "resolved/ref", *recorded.SecretRefPath,
		"the persisted SecretRefPath is resolved by reading the SecretReference back from OpenChoreo, never CreateSecret's bare name or a locally-computed path")
}

// TestAttemptProvision_AlreadyHasSecretRef_SkipsRecovery guards the inverse of
// the recovery case above: a binding that already has both a Thunder identity
// AND a previously stored secret must not regenerate or re-store on a
// subsequent attempt (e.g. a retry triggered by an unrelated transient
// failure elsewhere) — that would needlessly invalidate a secret that is
// already valid and possibly already in use.
func TestAttemptProvision_AlreadyHasSecretRef_SkipsRecovery(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		t.Fatal("CreateAgentIdentity must not be called when the binding already has a thunderAgentID")
		return "", "", "", false, nil
	}
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		t.Fatal("must not regenerate a secret when one is already stored")
		return "", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("must not store a new secret when one is already stored")
			return "", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ThunderAgentID: "already-created", ThunderClientID: "already-client-id", SecretRefPath: "existing/path",
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	require.NotNil(t, recorded.SecretRefPath, "an already-stored secret ref must be left untouched")
	assert.Equal(t, "existing/path", *recorded.SecretRefPath)
}

func TestAttemptProvision_ConflictFallback_RegeneratesSecretToRecover(t *testing.T) {
	// This models the partial-failure case from Section 6.8 of the architecture
	// doc: Thunder already has the agent (a prior attempt's DB write must have
	// failed after Thunder succeeded), so create falls back to a name lookup —
	// which never returns a secret. Recovery must regenerate one so the binding
	// ends up with a usable, storable secret instead of getting stuck forever.
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "thunder-agent-1", "client-abc", "", false, nil // created=false, no secret
	}
	regenerateCalled := false
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		regenerateCalled = true
		assert.Equal(t, "thunder-agent-1", thunderAgentID)
		return "recovered-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var storedSecret string
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			storedSecret = data[thundersvc.AgentSecretKeyClientSecret]
			return "ref", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
	}
	svc.AttemptProvision(context.Background(), binding)

	assert.True(t, regenerateCalled)
	assert.Equal(t, "recovered-secret", storedSecret)
	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
}

func TestAttemptProvision_TransientFailure_SchedulesRetryWithBackoff(t *testing.T) {
	boom := errors.New("connection refused")
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "", "", "", false, boom
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{}

	tests := []struct {
		name              string
		attemptCountSoFar int
		expectedDelay     time.Duration
	}{
		{"first failure -> 3 minutes", 0, 3 * time.Minute},
		{"second failure -> 3 minutes", 1, 3 * time.Minute},
		{"third failure -> 3 minutes", 2, 3 * time.Minute},
		{"fourth failure -> 3 minutes", 3, 3 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorded repositories.AgentThunderAttemptUpdate
			repo := &repomocks.AgentThunderClientRepositoryMock{
				ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
				UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
					recorded = fields
					return nil
				},
			}
			svc := newTestProvisioningService(repo, resolver, store)
			binding := models.AgentThunderClient{
				ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
				EnvironmentName: "staging", AttemptCount: tt.attemptCountSoFar,
			}

			before := time.Now()
			svc.AttemptProvision(context.Background(), binding)

			assert.Equal(t, models.AgentThunderStatusPending, recorded.Status,
				"a transient failure must stay PENDING for the reconciler to retry, not FAILED")
			assert.Equal(t, userFacingProvisioningError(boom), recorded.LastError,
				"LastError must be the sanitized user-facing message, never the raw cause text")
			require.NotNil(t, recorded.NextRetryAt)
			assert.WithinDuration(t, before.Add(tt.expectedDelay), *recorded.NextRetryAt, 2*time.Second)
		})
	}
}

func TestAttemptProvision_FifthFailure_MarksFailedNoMoreRetries(t *testing.T) {
	boom := errors.New("thunder unreachable")
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "", "", "", false, boom
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{}

	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	// 4 attempts already made; this is the 5th (and final, per the max-5 budget).
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", AttemptCount: 4,
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusFailed, recorded.Status)
	assert.Nil(t, recorded.NextRetryAt)
	assert.Equal(t, userFacingProvisioningError(boom), recorded.LastError,
		"LastError must be the sanitized user-facing message, never the raw cause text")
}

// TestProvisionBackoffSchedule_TotalRetryWindowWithinSLA guards the actual
// requirement behind the schedule: whatever the individual per-retry delay
// is, the cumulative delay before the 5th (final) attempt fires must stay
// within the 15-minute SLA for one binding's retry budget to resolve.
func TestProvisionBackoffSchedule_TotalRetryWindowWithinSLA(t *testing.T) {
	cumulative := time.Duration(maxProvisionAttempts-1) * provisionRetryDelay
	assert.LessOrEqualf(t, cumulative, 15*time.Minute,
		"cumulative delay before the final attempt (%s) must not exceed the 15-minute SLA", cumulative)
}

// TestAttemptProvision_ThunderNotProvisioned_RetriesLikeAnyOtherFailure guards
// against ErrThunderNotProvisioned being treated as an immediate permanent
// failure: an environment can exist before its (async) env-Thunder bootstrap
// has finished, so the first attempt seeing "not provisioned yet" must still
// go through the normal retry budget — not skip straight to FAILED, which
// would leave the binding stuck forever (the reconciler never retries FAILED
// rows, and ProvisionForEnvironmentIfMissing treats any existing row,
// including FAILED, as already provisioned).
func TestAttemptProvision_ThunderNotProvisioned_RetriesLikeAnyOtherFailure(t *testing.T) {
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) {
			return nil, thundersvc.ErrThunderNotProvisioned
		},
	}
	store := &clientmocks.SecretManagementClientMock{}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "no-thunder-env",
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusPending, recorded.Status)
	assert.NotNil(t, recorded.NextRetryAt)

	// After exhausting the retry budget, it does finally settle as FAILED —
	// same terminal outcome as before, just not on the very first attempt.
	binding.AttemptCount = maxProvisionAttempts - 1
	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusFailed, recorded.Status)
	assert.Nil(t, recorded.NextRetryAt)
	assert.Equal(t, userFacingProvisioningError(thundersvc.ErrThunderNotProvisioned), recorded.LastError,
		"once exhausted, the Overview UI reads LastError directly — it must be the sanitized message, not the sentinel's raw text")
}

// TestUserFacingProvisioningError_ClassifiesKnownCauses guards against
// leaking raw cause text (e.g. a SQLSTATE) into the UI-facing message.
func TestUserFacingProvisioningError_ClassifiesKnownCauses(t *testing.T) {
	tests := []struct {
		name  string
		cause error
	}{
		{"not provisioned", thundersvc.ErrThunderNotProvisioned},
		{"not provisioned, wrapped", fmt.Errorf("failed to read env-thunder url handle for acme/default: %w", thundersvc.ErrThunderNotProvisioned)},
		{"unreachable", thundersvc.ErrThunderUnreachable},
		{"unreachable, wrapped", fmt.Errorf("%w: default/staging", thundersvc.ErrThunderUnreachable)},
		{"opaque database error", fmt.Errorf(`failed to read env-thunder url handle for acme/default: ERROR: relation "env_thunder_urls" does not exist (SQLSTATE 42P01)`)},
		{"opaque thunder API error", errors.New("create agent identity: 500 Internal Server Error")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := userFacingProvisioningError(tt.cause)
			assert.NotEmpty(t, msg)
			assert.NotContains(t, msg, "SQLSTATE", "must never leak raw database detail")
			assert.NotContains(t, msg, "env_thunder_urls", "must never leak internal table/field names")
		})
	}

	assert.Equal(t,
		userFacingProvisioningError(thundersvc.ErrThunderNotProvisioned),
		userFacingProvisioningError(fmt.Errorf("wrapped: %w", thundersvc.ErrThunderNotProvisioned)),
		"wrapping must not change the classification")
	assert.NotEqual(t,
		userFacingProvisioningError(thundersvc.ErrThunderNotProvisioned),
		userFacingProvisioningError(thundersvc.ErrThunderUnreachable),
		"not-provisioned and unreachable point at different remediation and must read differently")
	assert.NotEqual(t,
		userFacingProvisioningError(thundersvc.ErrThunderUnreachable),
		userFacingProvisioningError(errors.New("some other opaque failure")),
		"an actual sentinel must not collapse to the same message as the generic fallback")
}

// TestAttemptProvision_Failure_LogsRawCauseButSanitizesLastError guards that
// sanitizing LastError doesn't also drop the raw cause from the logs.
func TestAttemptProvision_Failure_LogsRawCauseButSanitizesLastError(t *testing.T) {
	boom := errors.New(`ERROR: relation "env_thunder_urls" does not exist (SQLSTATE 42P01)`)
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return nil, boom },
	}
	store := &clientmocks.SecretManagementClientMock{}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	svc := NewAgentThunderProvisioningService(repo, resolver, store, testOCClientResolvingSecretRefs(), nil, logger)

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "default",
	})

	assert.Equal(t, userFacingProvisioningError(boom), recorded.LastError)
	assert.Contains(t, buf.String(), "SQLSTATE", "the raw cause must still reach the log for operators to diagnose")
	assert.Contains(t, buf.String(), "env_thunder_urls")
}

// TestAttemptProvision_ClaimFails_SkipsWithoutTouchingThunder guards the fix
// for the dual-concurrent-provisioning race: the inline fast-path goroutine
// and the reconciler's sweep can both land on the same freshly-written
// binding within the same ~60s window. ClaimForAttempt is the atomic gate
// that must be checked BEFORE any Thunder call or DB write — if it reports
// the binding is already claimed (e.g. a concurrent AttemptProvision call
// beat us to it), this call must be a complete no-op.
func TestAttemptProvision_ClaimFails_SkipsWithoutTouchingThunder(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		t.Fatal("CreateAgentIdentity must not be called when the claim was not won")
		return "", "", "", false, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) {
			t.Fatal("Resolve must not be called when the claim was not won")
			return tc, nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error {
			t.Fatal("UpdateAfterAttempt must not be called when the claim was not won")
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
	}

	svc.AttemptProvision(context.Background(), binding)
	// Reaching here without a t.Fatal means the no-op guard held.
}

// TestAttemptProvision_PanicIsRecovered_MarksBindingRetryable guards against a
// panic anywhere in one attempt (e.g. AgentThunderAppName on an invalid slug,
// or any future nil-deref) crashing the whole process — this runs on a
// detached goroutine or the reconciler's per-binding goroutine, so an
// unrecovered panic here takes down all in-flight requests, not just this
// one binding.
func TestAttemptProvision_PanicIsRecovered_MarksBindingRetryable(t *testing.T) {
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) {
			panic("simulated panic during provisioning")
		},
	}
	store := &clientmocks.SecretManagementClientMock{}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
	}

	assert.NotPanics(t, func() {
		svc.AttemptProvision(context.Background(), binding)
	}, "a panic during one attempt must not propagate and crash the process")

	assert.Equal(t, models.AgentThunderStatusPending, recorded.Status, "must be scheduled for retry, not left stuck in-progress")
	assert.NotNil(t, recorded.NextRetryAt)
	// Opaque, non-sentinel cause — always the generic branch, any text works.
	assert.Equal(t, userFacingProvisioningError(errors.New("anything")), recorded.LastError,
		"LastError must be the sanitized user-facing message, never the raw panic text")
}

func TestProvisionForAgent_WritesAheadPendingForEveryEnvironment(t *testing.T) {
	var upserted []models.AgentThunderClient
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpsertFunc: func(_ context.Context, c *models.AgentThunderClient) error {
			upserted = append(upserted, *c)
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) {
			// Block forever from the caller's perspective is unnecessary — just
			// return an error so the background attempt fails harmlessly; this
			// test only cares about the synchronous write-ahead behavior.
			return nil, errors.New("unused in this test")
		},
	}
	store := &clientmocks.SecretManagementClientMock{}
	repo.UpdateAfterAttemptFunc = func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error { return nil }

	svc := newTestProvisioningService(repo, resolver, store)
	svc.ProvisionForAgent(context.Background(), "acme", "proj1", "my-agent", models.AgentProvisioningTypeInternal, []string{"dev", "staging", "prod"}, "platform-thunder-subject-abc123")

	require.Len(t, upserted, 3, "must write one PENDING row per environment, synchronously, before returning")
	envs := []string{upserted[0].EnvironmentName, upserted[1].EnvironmentName, upserted[2].EnvironmentName}
	assert.ElementsMatch(t, []string{"dev", "staging", "prod"}, envs)
	for _, u := range upserted {
		assert.Equal(t, models.AgentThunderStatusPending, u.Status)
		assert.Equal(t, models.AgentProvisioningTypeInternal, u.ProvisioningType)
		assert.Equal(t, "platform-thunder-subject-abc123", u.RequestedBy,
			"the caller's identity must be captured on every environment's write-ahead row, for audit — never sent to Thunder itself")
	}
}

// TestProvisionForAgent_TransientUpsertBlip_RecoveredOnRetry guards against a
// momentary write-ahead DB error silently dropping an environment forever:
// the reconciler can only find and heal a binding that has a row, so the
// insert itself must tolerate a one-off blip.
func TestProvisionForAgent_TransientUpsertBlip_RecoveredOnRetry(t *testing.T) {
	attemptsForStaging := 0
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpsertFunc: func(_ context.Context, c *models.AgentThunderClient) error {
			if c.EnvironmentName == "staging" {
				attemptsForStaging++
				if attemptsForStaging < writeAheadUpsertAttempts {
					return errors.New("connection reset (transient)")
				}
			}
			return nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error { return nil },
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) {
			return nil, errors.New("unused in this test")
		},
	}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	svc.ProvisionForAgent(context.Background(), "acme", "proj1", "my-agent", models.AgentProvisioningTypeExternal, []string{"staging"}, "")

	assert.Equal(t, writeAheadUpsertAttempts, attemptsForStaging,
		"a transient Upsert failure must be retried, not silently drop the environment with no row for the reconciler to find")
}

// TestRegenerateSecret_Internal_StoresSecret guards internal agents' one
// storage requirement: their secret must persist so it can later be injected
// into the workload.
func TestRegenerateSecret_Internal_StoresSecret(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		assert.Equal(t, "thunder-agent-1", thunderAgentID)
		return "new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var storedSecret string
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			storedSecret = data[thundersvc.AgentSecretKeyClientSecret]
			return "ref", nil
		},
	}
	var updatedPath string
	bindingID := uuid.New()
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: bindingID, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeInternal,
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			updatedPath = secretRefPath
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	ownership, clientID, newSecret, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeInternal, ownership)
	assert.Equal(t, "client-abc", clientID)
	assert.Equal(t, "new-secret", newSecret)
	assert.Equal(t, "new-secret", storedSecret)
	assert.Equal(t, "resolved/ref", updatedPath,
		"the persisted SecretRefPath is resolved by reading the SecretReference back from OpenChoreo, never CreateSecret's bare name or a locally-computed path")
}

// TestRegenerateSecret_Internal_CreateSecretErrorPropagatesDirectly_NoInjector
// guards the no-workload-injector case: even when CreateSecret's error
// matches the "after create conflict" marker storeCredential otherwise
// retries on (see TestStoreCredential_ConflictMarker_CleansUpAndRetries),
// there is nothing to clean up without an injector configured, so the
// original error propagates on the first and only attempt instead of
// panicking or retrying blind. newTestProvisioningService (used here) wires
// no injector; newTestProvisioningServiceWithInjector is the variant that does.
func TestRegenerateSecret_Internal_CreateSecretErrorPropagatesDirectly_NoInjector(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(context.Context, string) (string, error) {
		return "new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var createCalls int
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			createCalls++
			return "", fmt.Errorf("failed to upsert secret: failed to update secret after create conflict: %w", utils.ErrNotFound)
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeInternal,
			}, nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrNotFound)
	assert.Equal(t, 1, createCalls, "no retry — the error propagates on the first and only attempt")
}

// TestStoreCredential_ConflictMarker_CleansUpAndRetries guards the actual
// recovery path the marker above exists for: an already-provisioned internal
// agent's stray SecretReference (owned by AgentIdentityInjectionService, not
// this code) collides with a fresh CreateSecret. With a workload injector
// configured, storeCredential clears that stray reference and retries once,
// succeeding on a now-unobstructed create.
func TestStoreCredential_ConflictMarker_CleansUpAndRetries(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(context.Context, string) (string, error) {
		return "new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var createCalls int
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			createCalls++
			if createCalls == 1 {
				return "", fmt.Errorf("failed to upsert secret: failed to update secret after create conflict: %w", utils.ErrNotFound)
			}
			return "ref", nil
		},
	}
	var cleanupCalls int
	injector := &agentIdentityInjectorStub{
		CleanupForEnvironmentFunc: func(_ context.Context, orgName, agentName, envName string) error {
			cleanupCalls++
			assert.Equal(t, "acme", orgName)
			assert.Equal(t, "my-agent", agentName)
			assert.Equal(t, "staging", envName)
			return nil
		},
	}
	var updatedPath string
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeInternal,
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			updatedPath = secretRefPath
			return nil
		},
	}

	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, injector)
	_, _, newSecret, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, "new-secret", newSecret)
	assert.Equal(t, 2, createCalls, "the first create hits the stray reference; the retry succeeds")
	assert.Equal(t, 1, cleanupCalls)
	assert.Equal(t, "resolved/ref", updatedPath)
}

// TestStoreCredential_ConflictMarker_CleanupFails_WrapsBothErrors guards the
// double-failure case: if the stray SecretReference can't even be cleaned up,
// the original CreateSecret error must not be silently dropped — both are
// wrapped together so the real cause (the cleanup failure blocking recovery)
// is visible alongside what triggered the recovery attempt in the first place.
func TestStoreCredential_ConflictMarker_CleanupFails_WrapsBothErrors(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(context.Context, string) (string, error) {
		return "new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var createCalls int
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			createCalls++
			return "", fmt.Errorf("failed to upsert secret: failed to update secret after create conflict: %w", utils.ErrNotFound)
		},
	}
	cleanupErr := errors.New("cleanup boom")
	injector := &agentIdentityInjectorStub{
		CleanupForEnvironmentFunc: func(context.Context, string, string, string) error { return cleanupErr },
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeInternal,
			}, nil
		},
	}

	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, injector)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.Error(t, err)
	assert.ErrorIs(t, err, cleanupErr)
	assert.ErrorIs(t, err, utils.ErrNotFound, "the original CreateSecret error must still be reachable, not replaced by the cleanup error")
	assert.Equal(t, 1, createCalls, "no retry attempted once cleanup itself failed")
}

// TestRegenerateSecret_External_NeverStoresSecret guards the invariant that an
// external agent's secret is minted by Thunder and handed straight back in
// the response — it must never touch secretMgmtClient at all, first time or
// any later rotation.
func TestRegenerateSecret_External_NeverStoresSecret(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		assert.Equal(t, "thunder-agent-1", thunderAgentID)
		return "new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("must not store a secret for an external agent")
			return "", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeExternal,
			}, nil
		},
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error {
			t.Fatal("must not record a secret ref for an external agent")
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	ownership, clientID, newSecret, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeExternal, ownership)
	assert.Equal(t, "client-abc", clientID)
	assert.Equal(t, "new-secret", newSecret, "the minted secret is still returned directly, just never persisted")
}

// TestRegenerateSecret_ConcurrentCallsForSameBinding_Serialized guards against
// interleaving two rotations of the same binding: without a per-binding lock,
// two concurrent RegenerateSecret calls could rotate Thunder in one order but
// write to OpenBao in the other, leaving the stored secret mismatched with
// whatever Thunder actually considers active.
func TestRegenerateSecret_ConcurrentCallsForSameBinding_Serialized(t *testing.T) {
	var inFlight int32
	var maxObservedInFlight int32
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			max := atomic.LoadInt32(&maxObservedInFlight)
			if n <= max || atomic.CompareAndSwapInt32(&maxObservedInFlight, max, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // widen the race window
		atomic.AddInt32(&inFlight, -1)
		return "rotated-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeInternal,
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}

	svc := newTestProvisioningService(repo, resolver, store)
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, atomic.LoadInt32(&maxObservedInFlight),
		"concurrent RegenerateSecret calls for the same binding must be serialized, not interleaved")
}

func TestRegenerateSecret_NotYetProvisioned_Errors(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

// TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500 guards a
// distinct case from the one above: no row exists at all yet (as opposed to a
// row existing with an empty ThunderAgentID). Before the fix, this returned
// the raw repositories.ErrAgentThunderClientNotFound unwrapped, which
// handleCommonErrors has no case for — resulting in an HTTP 500 for a
// routine, foreseeable caller state (e.g. calling regenerate immediately
// after CreateAgent, before the async provisioning goroutine has run).
func TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

func TestRevokeSecret_RotatesAndClearsStoredCopy(t *testing.T) {
	tc := fakeThunderClientMock()
	rotateCalled := false
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		rotateCalled = true
		return "unused-new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	deleteCalled := false
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ string) error {
			deleteCalled = true
			assert.Equal(t, "acme", location.OrgName)
			return nil
		},
	}
	var clearedTo string
	clearedToSet := false
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc", SecretRefPath: "existing/path",
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			clearedTo = secretRefPath
			clearedToSet = true
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, "client-abc", clientID, "revoke must return the (unchanged) client ID so the caller can build a response body instead of an empty 204")
	assert.True(t, rotateCalled, "revoke must invalidate the old secret in Thunder")
	assert.True(t, deleteCalled, "revoke must remove the stale stored copy, not leave an invalid secret sitting in the vault")
	require.True(t, clearedToSet)
	assert.Empty(t, clearedTo, "after revoke there must be no currently-valid stored secret until an explicit regenerate")
}

// TestRevokeSecret_StoredSecretDeleteFails_LeavesSecretRefPathSetForRetry
// guards against silently orphaning a stored secret: if deleteCredential
// fails, UpdateSecretRef must NOT run at all, so secret_ref_path stays
// pointing at the (still-present) stored copy — otherwise a re-revoke later
// would see an already-empty path and never retry the delete, permanently
// losing track of it.
func TestRevokeSecret_StoredSecretDeleteFails_LeavesSecretRefPathSetForRetry(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) { return "unused-new-secret", nil }
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ string) error {
			return errors.New("openbao unavailable")
		},
	}
	updateSecretRefCalled := false
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc", SecretRefPath: "existing/path",
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
			updateSecretRefCalled = true
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err, "the Thunder-side rotation already succeeded — a storage cleanup failure must not fail the whole revoke")
	assert.Equal(t, "client-abc", clientID)
	assert.False(t, updateSecretRefCalled,
		"secret_ref_path must be left untouched when the stored copy could not be deleted, so a later re-revoke retries it instead of losing track of the orphaned secret")
}

func TestRevokeSecret_NotYetProvisioned_Errors(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	assert.Empty(t, clientID)
}

// TestRevokeSecret_NoBindingRow_ReturnsNotProvisioned_Not500 is the Revoke
// counterpart to TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500
// — see that test's comment for why the no-row case is distinct and was
// previously unhandled.
func TestRevokeSecret_NoBindingRow_ReturnsNotProvisioned_Not500(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	assert.Empty(t, clientID)
}

func TestGetAgentRoles_Success(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "thunder-agent-1"}, nil
		},
	}
	wantRoles := []thundersvc.ThunderRole{{ID: "role-1", Name: "reader"}}
	identityClient := &clientmocks.EnvIdentityClientMock{
		GetAgentRolesFunc: func(_ context.Context, agentID string) ([]thundersvc.ThunderRole, error) {
			assert.Equal(t, "thunder-agent-1", agentID)
			return wantRoles, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, _, _, _ string) (thundersvc.EnvIdentityClient, error) {
			return identityClient, nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	roles, err := svc.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.NoError(t, err)
	assert.Equal(t, wantRoles, roles)
}

func TestGetAgentRoles_NotYetProvisioned_Errors(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, err := svc.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

// TestGetAgentRoles_NoBindingRow_ReturnsNotProvisioned_Not500 guards the same
// distinction as TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500:
// no row exists at all yet, as opposed to a row existing with an empty
// ThunderAgentID — both must map to the same sentinel, not an unwrapped
// repository error that handleCommonErrors has no case for.
func TestGetAgentRoles_NoBindingRow_ReturnsNotProvisioned_Not500(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, err := svc.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

func TestGetAgentGroups_Success(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "thunder-agent-1"}, nil
		},
	}
	wantGroups := []thundersvc.ThunderGroup{{ID: "group-1", Name: "operators"}}
	identityClient := &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(_ context.Context) (string, error) { return "ou-1", nil },
		GetAgentGroupsFunc: func(_ context.Context, ouID, agentID string) ([]thundersvc.ThunderGroup, error) {
			assert.Equal(t, "ou-1", ouID)
			assert.Equal(t, "thunder-agent-1", agentID)
			return wantGroups, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, _, _, _ string) (thundersvc.EnvIdentityClient, error) {
			return identityClient, nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	groups, err := svc.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.NoError(t, err)
	assert.Equal(t, wantGroups, groups)
}

func TestGetAgentGroups_NotYetProvisioned_Errors(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, err := svc.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

// TestGetAgentGroups_NoBindingRow_ReturnsNotProvisioned_Not500 is the groups
// counterpart to TestGetAgentRoles_NoBindingRow_ReturnsNotProvisioned_Not500.
func TestGetAgentGroups_NoBindingRow_ReturnsNotProvisioned_Not500(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, err := svc.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

// TestGetIdentityViews_NeverTouchesSecretStore guards GetIdentityViews'
// contract as a safe, non-destructive read: it must never read or delete
// anything from the secret store, even for an internal binding that does
// have a stored secret.
func TestGetIdentityViews_NeverTouchesSecretStore(t *testing.T) {
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, string) error {
			t.Fatal("GetIdentityViews must never destroy a secret")
			return nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: uuid.New(), EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-uuid-1", ThunderClientID: "client-abc",
				SecretRefPath: "some/path",
			}}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")

	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "thunder-agent-uuid-1", views[0].AgentID, "the view must expose Thunder's own identity UUID, not just the OAuth client_id")
}

func TestGetIdentityViews_External_ExposesClientIDNoSecret(t *testing.T) {
	store := &clientmocks.SecretManagementClientMock{}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: uuid.New(), EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderClientID: "client-abc",
			}}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")

	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "client-abc", views[0].ClientID)
}

func TestGetIdentityViews_Internal_SecretNeverReturned(t *testing.T) {
	store := &clientmocks.SecretManagementClientMock{}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: uuid.New(), EnvironmentName: "prod", ProvisioningType: models.AgentProvisioningTypeInternal,
				Status: models.AgentThunderStatusCompleted, ThunderClientID: "client-xyz",
				SecretRefPath: "internal/secret/path", RequestedBy: "platform-thunder-subject-abc123",
			}}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")

	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "client-xyz", views[0].ClientID)
	assert.Equal(t, "platform-thunder-subject-abc123", views[0].RequestedBy,
		"the audit trail must be visible via GET .../identity regardless of ownership type")
}

func TestDeleteAllBindings_DeletesThunderIdentitiesSecretsAndRows(t *testing.T) {
	tc := fakeThunderClientMock()
	var deletedIdentities []string
	tc.DeleteAgentIdentityFunc = func(_ context.Context, thunderAgentID string) (bool, error) {
		deletedIdentities = append(deletedIdentities, thunderAgentID)
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var deletedSecretEnvs []string
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ string) error {
			deletedSecretEnvs = append(deletedSecretEnvs, location.EnvironmentName)
			return nil
		},
	}
	devID, prodID, neverProvisionedID := uuid.New(), uuid.New(), uuid.New()
	seed := []models.AgentThunderClient{
		{ID: devID, ThunderAgentID: "agent-in-dev", EnvironmentName: "dev", SecretRefPath: "path/dev"},
		{ID: prodID, ThunderAgentID: "agent-in-prod", EnvironmentName: "prod", SecretRefPath: "path/prod"},
		{ID: neverProvisionedID, ThunderAgentID: "", EnvironmentName: "never-provisioned"}, // never made it to Thunder — nothing to delete there
	}
	var deletedIDs []uuid.UUID
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return seed, nil
		},
		GetFunc: func(_ context.Context, _, _, _, envName string) (*models.AgentThunderClient, error) {
			for _, b := range seed {
				if b.EnvironmentName == envName {
					return &b, nil
				}
			}
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		DeleteByIDsFunc: func(_ context.Context, ids []uuid.UUID) error {
			deletedIDs = ids
			return nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}

	svc := newTestProvisioningService(repo, resolver, store)
	svc.DeleteAllBindings(context.Background(), "acme", "proj1", "my-agent")

	assert.ElementsMatch(t, []string{"agent-in-dev", "agent-in-prod"}, deletedIdentities)
	assert.ElementsMatch(t, []string{"dev", "prod"}, deletedSecretEnvs)
	// Deletion must be scoped to exactly the rows snapshotted above (by ID), not
	// a blanket delete-by-agent-name — otherwise a concurrent recreate of the
	// same agent name could have its fresh rows swept up by this call too.
	assert.ElementsMatch(t, []uuid.UUID{devID, prodID, neverProvisionedID}, deletedIDs)
}

// TestDeleteAllBindings_DeletesRowsBeforeSlowThunderCleanup guards against a
// narrower version of the same recreate race: even with ID-scoped deletion,
// deleting the rows only AFTER the (slow, per-environment) Thunder identity
// cleanup leaves a wide window where a same-named agent recreated in the
// meantime silently gets no fresh write-ahead row at all — Upsert's
// OnConflict DoNothing sees the still-present old row and no-ops. The DB
// rows must be deleted first, immediately after the snapshot.
func TestDeleteAllBindings_DeletesRowsBeforeSlowThunderCleanup(t *testing.T) {
	var order []string
	tc := fakeThunderClientMock()
	tc.DeleteAgentIdentityFunc = func(_ context.Context, _ string) (bool, error) {
		order = append(order, "thunder-delete")
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{}
	seed := models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "agent-1", EnvironmentName: "dev"}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{seed}, nil
		},
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &seed, nil
		},
		DeleteByIDsFunc: func(_ context.Context, _ []uuid.UUID) error {
			order = append(order, "db-delete")
			return nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}

	svc := newTestProvisioningService(repo, resolver, store)
	svc.DeleteAllBindings(context.Background(), "acme", "proj1", "my-agent")

	require.Equal(t, []string{"db-delete", "thunder-delete"}, order,
		"rows must be deleted before the slow per-environment Thunder cleanup, not after")
}

// TestDeleteAllBindings_StopsOnDeleteByIDsFailure guards against reopening the
// same recreate race DeleteByIDs-by-snapshot was introduced to close: if the DB
// delete fails, the local row is still present, so proceeding to delete the
// Thunder identity/secret below would leave that row pointing at now-destroyed
// resources — and a same-named agent recreated afterward would silently no-op
// against it via Upsert's OnConflict DoNothing.
// TestDeleteAllBindings_ContinuesExternalCleanupOnDeleteByIDsFailure documents
// a deliberate behavior change: this used to abort ALL cleanup (including
// live Thunder identities, SecretReference CRs, and OpenBao secrets) the
// moment the local DB row delete failed. That left external, still-active
// resources orphaned indefinitely — worse than a few stale local rows, since
// an orphaned Thunder identity can still mint valid tokens. Failing the DB
// delete must now be logged and treated as non-fatal, falling through to
// clean up everything else regardless.
func TestDeleteAllBindings_ContinuesExternalCleanupOnDeleteByIDsFailure(t *testing.T) {
	thunderDeleted := false
	tc := fakeThunderClientMock()
	tc.DeleteAgentIdentityFunc = func(_ context.Context, _ string) (bool, error) {
		thunderDeleted = true
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	secretDeleted := false
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, string) error {
			secretDeleted = true
			return nil
		},
	}
	bindingID := uuid.New()
	seed := models.AgentThunderClient{ID: bindingID, ThunderAgentID: "agent-1", EnvironmentName: "dev", SecretRefPath: "path/dev"}
	var clearedSecretRefIDs []uuid.UUID
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{seed}, nil
		},
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &seed, nil
		},
		DeleteByIDsFunc: func(_ context.Context, _ []uuid.UUID) error {
			return errors.New("transient db error")
		},
		UpdateSecretRefFunc: func(_ context.Context, id uuid.UUID, secretRefPath string) error {
			assert.Equal(t, "", secretRefPath)
			clearedSecretRefIDs = append(clearedSecretRefIDs, id)
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	svc.DeleteAllBindings(context.Background(), "acme", "proj1", "my-agent")

	assert.True(t, thunderDeleted, "a failed local DB row delete must not block deleting the live Thunder identity")
	assert.True(t, secretDeleted, "a failed local DB row delete must not block deleting the OpenBao secret")
	assert.Equal(t, []uuid.UUID{bindingID}, clearedSecretRefIDs,
		"secret_ref_path must be cleared even when the row-delete itself fails, so the surviving row can't be "+
			"picked up by the reconciler's FindRecentlyCompletedInternal sweep and re-injected against already-deleted resources")
}

// TestDeleteAllBindings_WaitsForInFlightAttemptProvision_NoOrphanedThunderIdentity
// guards the race the per-binding lock in DeleteAllBindings exists to close: a
// concurrent AttemptProvision already holding the SAME binding's lock finishes
// AFTER DeleteAllBindings has snapshotted the (still not-yet-provisioned) row
// via FindByAgent. Without waiting for that lock and re-fetching, the freshly
// minted Thunder identity and OpenBao secret the attempt just wrote would
// never be cleaned up here — orphaned, and still able to mint valid tokens.
func TestDeleteAllBindings_WaitsForInFlightAttemptProvision_NoOrphanedThunderIdentity(t *testing.T) {
	bindingID := uuid.New()
	const ouID, projectName, agentName, envName = "acme", "proj1", "my-agent", "dev"

	// row stands in for the real DB row: both AttemptProvision (via
	// UpdateAfterAttempt) and DeleteAllBindings (via Get) observe it. Its
	// OUID/ProjectName/AgentName must match what DeleteAllBindings is called
	// with below — bindingLockKey is built from these fields, so a mismatch
	// here would silently give AttemptProvision and DeleteAllBindings two
	// DIFFERENT locks instead of making them contend for the same one,
	// defeating the entire point of this test.
	var mu sync.Mutex
	row := models.AgentThunderClient{
		ID: bindingID, OUID: ouID, ProjectName: projectName, AgentName: agentName,
		EnvironmentName: envName, ProvisioningType: models.AgentProvisioningTypeInternal,
	}

	attemptStarted := make(chan struct{})
	releaseAttempt := make(chan struct{})

	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		close(attemptStarted)
		<-releaseAttempt // held until the test confirms DeleteAllBindings is blocked on the lock
		return "thunder-agent-1", "client-abc", "secret-xyz", true, nil
	}
	var deletedThunderIDs []string
	tc.DeleteAgentIdentityFunc = func(_ context.Context, thunderAgentID string) (bool, error) {
		deletedThunderIDs = append(deletedThunderIDs, thunderAgentID)
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var deletedSecretEnvs []string
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref", nil
		},
		DeleteSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ string) error {
			deletedSecretEnvs = append(deletedSecretEnvs, location.EnvironmentName)
			return nil
		},
	}

	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			mu.Lock()
			defer mu.Unlock()
			snapshot := row // stale: still not-yet-provisioned at snapshot time
			return []models.AgentThunderClient{snapshot}, nil
		},
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			mu.Lock()
			defer mu.Unlock()
			current := row
			return &current, nil
		},
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, update repositories.AgentThunderAttemptUpdate) error {
			mu.Lock()
			defer mu.Unlock()
			row.Status = update.Status
			if update.ThunderAgentID != nil {
				row.ThunderAgentID = *update.ThunderAgentID
			}
			if update.ThunderClientID != nil {
				row.ThunderClientID = *update.ThunderClientID
			}
			if update.SecretRefPath != nil {
				row.SecretRefPath = *update.SecretRefPath
			}
			return nil
		},
		DeleteByIDsFunc:     func(_ context.Context, _ []uuid.UUID) error { return nil },
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}

	svc := newTestProvisioningService(repo, resolver, store)

	go svc.AttemptProvision(context.Background(), row)
	select {
	case <-attemptStarted: // AttemptProvision now holds the binding lock, mid-Thunder-call
	case <-time.After(2 * time.Second):
		t.Fatal("AttemptProvision never reached CreateAgentIdentity — check the mocked call chain, not the lock behavior under test")
	}

	deleteDone := make(chan struct{})
	go func() {
		svc.DeleteAllBindings(context.Background(), ouID, projectName, agentName)
		close(deleteDone)
	}()

	select {
	case <-deleteDone:
		close(releaseAttempt) // unblock the leaked AttemptProvision goroutine before failing
		t.Fatal("DeleteAllBindings must block on the in-flight attempt's lock, not race ahead of it")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseAttempt) // let the attempt finish and write the real Thunder identity + secret

	select {
	case <-deleteDone:
	case <-time.After(2 * time.Second):
		t.Fatal("DeleteAllBindings never completed after the in-flight attempt released the lock")
	}

	assert.Equal(t, []string{"thunder-agent-1"}, deletedThunderIDs,
		"the Thunder identity the concurrent attempt just created must be cleaned up, not orphaned")
	assert.Equal(t, []string{"dev"}, deletedSecretEnvs,
		"the OpenBao secret the concurrent attempt just stored must be cleaned up, not orphaned")
}

// TestDeleteAllBindings_ReleasesEachBindingsLockAfterItsOwnCleanup_NotAfterAll
// guards against re-widening each binding's lock hold time back to "held
// until every other binding's cleanup also finishes": two bindings, same
// agent, different environments, processed fast-one-first. The fast one's
// Thunder delete call returns immediately; the slow one's blocks
// indefinitely. While DeleteAllBindings is still stuck cleaning up the slow
// (second) binding, the fast (first) binding's lock must already be free to
// re-acquire — proving each binding's lock releases as soon as ITS OWN
// cleanup finishes rather than being held until the whole method returns,
// even though each binding's own lock still guards its own full lifecycle
// (DB re-fetch through its own external cleanup) end-to-end.
func TestDeleteAllBindings_ReleasesEachBindingsLockAfterItsOwnCleanup_NotAfterAll(t *testing.T) {
	const ouID, projectName, agentName = "acme", "proj1", "my-agent"
	const slowEnv, fastEnv = "slow-env", "fast-env"

	rows := map[string]models.AgentThunderClient{
		slowEnv: {
			ID: uuid.New(), OUID: ouID, ProjectName: projectName, AgentName: agentName,
			EnvironmentName: slowEnv, ProvisioningType: models.AgentProvisioningTypeInternal,
			ThunderAgentID: "thunder-slow", SecretRefPath: "path/slow",
		},
		fastEnv: {
			ID: uuid.New(), OUID: ouID, ProjectName: projectName, AgentName: agentName,
			EnvironmentName: fastEnv, ProvisioningType: models.AgentProvisioningTypeInternal,
			ThunderAgentID: "thunder-fast", SecretRefPath: "path/fast",
		},
	}

	slowDeleteStarted := make(chan struct{})
	releaseSlowDelete := make(chan struct{})
	tc := fakeThunderClientMock()
	tc.DeleteAgentIdentityFunc = func(_ context.Context, thunderAgentID string) (bool, error) {
		if thunderAgentID == "thunder-slow" {
			close(slowDeleteStarted)
			<-releaseSlowDelete
		}
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, string) error { return nil },
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{rows[fastEnv], rows[slowEnv]}, nil
		},
		GetFunc: func(_ context.Context, _, _, _, envName string) (*models.AgentThunderClient, error) {
			row := rows[envName]
			return &row, nil
		},
		DeleteByIDsFunc:     func(_ context.Context, _ []uuid.UUID) error { return nil },
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}
	svcIface := newTestProvisioningService(repo, resolver, store)
	svc, ok := svcIface.(*agentThunderProvisioningService)
	require.True(t, ok)

	done := make(chan struct{})
	go func() {
		svc.DeleteAllBindings(context.Background(), ouID, projectName, agentName)
		close(done)
	}()

	select {
	case <-slowDeleteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("slow binding's Thunder delete never started — check the mocked call chain, not the lock behavior under test")
	}

	// The slow binding's cleanup is still blocked — the fast binding's lock
	// must already be free: re-acquiring it here must not block.
	acquired := make(chan func(), 1)
	go func() {
		release, err := svc.bindingLocks.Lock(context.Background(), bindingLockKey(ouID, projectName, agentName, fastEnv))
		if err != nil {
			return
		}
		acquired <- release
	}()
	select {
	case release := <-acquired:
		release()
	case <-time.After(200 * time.Millisecond):
		close(releaseSlowDelete) // unblock the leaked goroutine before failing
		t.Fatal("fast binding's lock is still held while the slow binding's cleanup is in flight — locks are not being released per-binding")
	}

	close(releaseSlowDelete)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DeleteAllBindings never completed after the slow cleanup was released")
	}
}

// TestProvisionForEnvironmentIfMissing_* cover the shared helper behind both the
// external-agent identity-provision endpoint and PromoteAgent's internal-agent
// hook: an environment that appeared (or entered the pipeline) after the agent
// was first created still gets an AgentID once it's actually needed there.

func TestProvisionForEnvironmentIfMissing_AlreadyExists_LeavesBindingUntouched(t *testing.T) {
	getCalled := false
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, org, project, agent, env string) (*models.AgentThunderClient, error) {
			getCalled = true
			assert.Equal(t, "acme", org)
			assert.Equal(t, "proj1", project)
			assert.Equal(t, "my-agent", agent)
			assert.Equal(t, "new-env", env)
			return &models.AgentThunderClient{ID: uuid.New(), Status: models.AgentThunderStatusCompleted}, nil
		},
		UpsertFunc: func(_ context.Context, _ *models.AgentThunderClient) error {
			t.Fatal("must not write a new binding when one already exists")
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	alreadyExisted, err := svc.ProvisionForEnvironmentIfMissing(
		context.Background(), "acme", "proj1", "my-agent", "new-env", models.AgentProvisioningTypeExternal, "user-1",
	)

	require.NoError(t, err)
	assert.True(t, alreadyExisted)
	assert.True(t, getCalled)
}

func TestProvisionForEnvironmentIfMissing_Missing_WritesAheadAndAttempts(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	var upserted *models.AgentThunderClient
	upsertDone := make(chan struct{})
	repo.UpsertFunc = func(_ context.Context, c *models.AgentThunderClient) error {
		upserted = c
		close(upsertDone)
		return nil
	}
	// The background attempt (triggered by the missing-binding path) will run and
	// fail fast against the resolver below — give it somewhere harmless to land
	// instead of panicking on a nil mock function.
	repo.ClaimForAttemptFunc = func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }
	repo.UpdateAfterAttemptFunc = func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error { return nil }
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) {
			return nil, thundersvc.ErrThunderNotProvisioned // attempt fails fast; not under test here
		},
	}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	alreadyExisted, err := svc.ProvisionForEnvironmentIfMissing(
		context.Background(), "acme", "proj1", "my-agent", "new-env", models.AgentProvisioningTypeInternal, "user-1",
	)

	require.NoError(t, err)
	assert.False(t, alreadyExisted)

	select {
	case <-upsertDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected a write-ahead binding to be upserted for the missing environment")
	}
	require.NotNil(t, upserted)
	assert.Equal(t, "new-env", upserted.EnvironmentName)
	assert.Equal(t, models.AgentProvisioningTypeInternal, upserted.ProvisioningType)
	assert.Equal(t, "user-1", upserted.RequestedBy)
}

func TestProvisionForEnvironmentIfMissing_RepoErrorPropagates(t *testing.T) {
	boom := errors.New("connection refused")
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, boom
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.SecretManagementClientMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, err := svc.ProvisionForEnvironmentIfMissing(
		context.Background(), "acme", "proj1", "my-agent", "new-env", models.AgentProvisioningTypeExternal, "user-1",
	)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrAgentThunderClientNotFound), "a real repo error must not be mistaken for not-found")
}

// TestKeyedMutex_SerializesSameKey guards the core purpose of keyedMutex:
// RegenerateSecret/RevokeSecret for the SAME binding must never run
// interleaved, even from separate goroutines.
func TestKeyedMutex_SerializesSameKey(t *testing.T) {
	var m keyedMutex
	var active int32
	var maxObservedActive int32
	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			unlock, err := m.Lock(context.Background(), "same-key")
			assert.NoError(t, err)
			defer unlock()
			n := atomic.AddInt32(&active, 1)
			for {
				max := atomic.LoadInt32(&maxObservedActive)
				if n <= max || atomic.CompareAndSwapInt32(&maxObservedActive, max, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, atomic.LoadInt32(&maxObservedActive), "concurrent Lock calls for the same key must be serialized")
}

// TestKeyedMutex_DifferentKeysDoNotBlock guards against keyedMutex
// accidentally serializing unrelated bindings — only the same key should
// exclude, not a single process-wide lock.
func TestKeyedMutex_DifferentKeysDoNotBlock(t *testing.T) {
	var m keyedMutex
	unlockA, err := m.Lock(context.Background(), "key-a")
	require.NoError(t, err)
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB, err := m.Lock(context.Background(), "key-b")
		assert.NoError(t, err)
		defer unlockB()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Lock on a different key must not block behind an unrelated key's holder")
	}
}

// TestKeyedMutex_EvictsEntryAfterUnlock guards against the exact leak found in
// review: every distinct key that has ever been locked must not permanently
// occupy an entry in the map — once the last (and only) holder releases, the
// entry must be removed so the map doesn't grow unbounded with the number of
// bindings ever rotated over a long-lived process.
func TestKeyedMutex_EvictsEntryAfterUnlock(t *testing.T) {
	var m keyedMutex

	for i := range 100 {
		key := fmt.Sprintf("binding-%d", i)
		unlock, err := m.Lock(context.Background(), key)
		require.NoError(t, err)
		unlock()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Empty(t, m.locks, "every key's entry must be evicted once its last holder unlocks, not accumulate forever")
}

// TestKeyedMutex_EvictionSafeUnderConcurrentReacquire guards the trickiest
// part of refcounted eviction: a new Lock for the same key arriving exactly
// as the previous holder is releasing must never observe a torn/deleted
// entry it can proceed through concurrently with the still-finishing holder —
// otherwise eviction would silently reopen the very race keyedMutex exists to
// prevent.
func TestKeyedMutex_EvictionSafeUnderConcurrentReacquire(t *testing.T) {
	var m keyedMutex
	var active int32
	var maxObservedActive int32
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(iterations)
	for range iterations {
		go func() {
			defer wg.Done()
			unlock, err := m.Lock(context.Background(), "hot-key")
			assert.NoError(t, err)
			n := atomic.AddInt32(&active, 1)
			for {
				max := atomic.LoadInt32(&maxObservedActive)
				if n <= max || atomic.CompareAndSwapInt32(&maxObservedActive, max, n) {
					break
				}
			}
			atomic.AddInt32(&active, -1)
			unlock()
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, atomic.LoadInt32(&maxObservedActive), "eviction must never let two holders of the same key run concurrently")
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Empty(t, m.locks, "the key must be evicted once every holder has released")
}

// TestKeyedMutex_LockReturnsPromptlyWhenCtxDoneWhileWaiting guards the actual
// harm behind holding this lock across Thunder/OpenBao I/O: a waiter must not
// be stuck for as long as the current holder's I/O takes just because ITS OWN
// context says to give up sooner. Without ctx-awareness, this waiter would
// block until unlockHeld() below is called instead of returning immediately.
func TestKeyedMutex_LockReturnsPromptlyWhenCtxDoneWhileWaiting(t *testing.T) {
	var m keyedMutex
	unlockHeld, err := m.Lock(context.Background(), "contended-key")
	require.NoError(t, err)
	defer unlockHeld()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done — this waiter must never actually block

	done := make(chan struct{})
	var waitErr error
	go func() {
		_, waitErr = m.Lock(ctx, "contended-key")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Lock must return as soon as ctx is done, not wait for the current holder to release")
	}
	assert.ErrorIs(t, waitErr, context.Canceled)
}

// TestKeyedMutex_EvictsEntryAfterCtxCancelWhileWaiting guards that a waiter
// who gave up via ctx cancellation still gets its refcount bump cleaned up —
// otherwise the map entry would leak forever once the actual holder releases.
func TestKeyedMutex_EvictsEntryAfterCtxCancelWhileWaiting(t *testing.T) {
	var m keyedMutex
	unlockHeld, err := m.Lock(context.Background(), "contended-key")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, waitErr := m.Lock(ctx, "contended-key")
	require.ErrorIs(t, waitErr, context.Canceled)

	unlockHeld()

	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Empty(t, m.locks, "a cancelled waiter's refcount bump must not leak the map entry once the real holder releases")
}

// agentIdentityInjectorStub is a hand-written func-field stub for the
// in-package AgentIdentityInjectionService interface (no moq mock — see the
// service-unit-test conventions). A nil func field panics when called, so
// tests can prove a path is never reached, mirroring moq semantics.
type agentIdentityInjectorStub struct {
	EnvVarsForEnvironmentFunc   func(ctx context.Context, orgName, projectName, agentName, envName string) ([]client.EnvVar, error)
	InjectForEnvironmentFunc    func(ctx context.Context, orgName, projectName, agentName, envName string) error
	ReconcileForEnvironmentFunc func(ctx context.Context, orgName, projectName, agentName, envName string) error
	RefreshAfterRotationFunc    func(ctx context.Context, orgName, projectName, agentName, envName string) error
	RemoveForEnvironmentFunc    func(ctx context.Context, orgName, projectName, agentName, envName string, includeWorkloadLevel bool) error
	CleanupForEnvironmentFunc   func(ctx context.Context, orgName, agentName, envName string) error
}

func (s *agentIdentityInjectorStub) EnvVarsForEnvironment(ctx context.Context, orgName, projectName, agentName, envName string) ([]client.EnvVar, error) {
	if s.EnvVarsForEnvironmentFunc == nil {
		panic("unexpected call to EnvVarsForEnvironment")
	}
	return s.EnvVarsForEnvironmentFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *agentIdentityInjectorStub) InjectForEnvironment(ctx context.Context, orgName, projectName, agentName, envName string) error {
	if s.InjectForEnvironmentFunc == nil {
		panic("unexpected call to InjectForEnvironment")
	}
	return s.InjectForEnvironmentFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *agentIdentityInjectorStub) ReconcileForEnvironment(ctx context.Context, orgName, projectName, agentName, envName string) error {
	if s.ReconcileForEnvironmentFunc == nil {
		panic("unexpected call to ReconcileForEnvironment")
	}
	return s.ReconcileForEnvironmentFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *agentIdentityInjectorStub) RefreshAfterRotation(ctx context.Context, orgName, projectName, agentName, envName string) error {
	if s.RefreshAfterRotationFunc == nil {
		panic("unexpected call to RefreshAfterRotation")
	}
	return s.RefreshAfterRotationFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *agentIdentityInjectorStub) RemoveForEnvironment(ctx context.Context, orgName, projectName, agentName, envName string, includeWorkloadLevel bool) error {
	if s.RemoveForEnvironmentFunc == nil {
		panic("unexpected call to RemoveForEnvironment")
	}
	return s.RemoveForEnvironmentFunc(ctx, orgName, projectName, agentName, envName, includeWorkloadLevel)
}

func (s *agentIdentityInjectorStub) CleanupForEnvironment(ctx context.Context, orgName, agentName, envName string) error {
	if s.CleanupForEnvironmentFunc == nil {
		panic("unexpected call to CleanupForEnvironment")
	}
	return s.CleanupForEnvironmentFunc(ctx, orgName, agentName, envName)
}

// successfulProvisionMocks builds the resolver/store/repo trio for a
// clean-path AttemptProvision run, capturing the recorded update.
func successfulProvisionMocks(recorded *repositories.AgentThunderAttemptUpdate) (
	*repomocks.AgentThunderClientRepositoryMock,
	*clientmocks.EnvThunderResolverMock,
	*clientmocks.SecretManagementClientMock,
) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "thunder-agent-1", "client-abc", "secret-xyz", true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			*recorded = fields
			return nil
		},
	}
	return repo, resolver, store
}

func TestAttemptProvision_Success_InternalAgent_InjectsWorkloadCredentials(t *testing.T) {
	var recorded repositories.AgentThunderAttemptUpdate
	repo, resolver, store := successfulProvisionMocks(&recorded)

	reconciledCalls := 0
	injector := &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(ctx context.Context, orgName, projectName, agentName, envName string) error {
			reconciledCalls++
			assert.Equal(t, "acme", orgName)
			assert.Equal(t, "proj1", projectName)
			assert.Equal(t, "my-agent", agentName)
			assert.Equal(t, "staging", envName)
			resolvedOrg, ok := orgctx.GetResolvedOrg(ctx)
			require.True(t, ok, "workload reconciliation must carry an organization context")
			assert.Equal(t, "acme", resolvedOrg.OUID)
			return nil
		},
	}
	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, injector)

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	})

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	assert.Equal(t, 1, reconciledCalls, "successful internal provisioning must reconcile the workload's identity credentials exactly once")
}

// TestSetWorkloadInjector_BackfillsAfterConstruction proves the exact pattern
// app.Run relies on: a service constructed with no injector (because it must
// exist before the OpenChoreo client this dependency needs), backfilled via
// SetWorkloadInjector once that client is available, and used correctly by
// every AttemptProvision call from then on.
func TestSetWorkloadInjector_BackfillsAfterConstruction(t *testing.T) {
	var recorded repositories.AgentThunderAttemptUpdate
	repo, resolver, store := successfulProvisionMocks(&recorded)
	svc := newTestProvisioningService(repo, resolver, store)

	setter, ok := svc.(WorkloadInjectorSetter)
	require.True(t, ok, "the concrete provisioning service must implement the optional WorkloadInjectorSetter interface")

	reconciledCalls := 0
	setter.SetWorkloadInjector(&agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(context.Context, string, string, string, string) error {
			reconciledCalls++
			return nil
		},
	})

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	})

	assert.Equal(t, 1, reconciledCalls, "AttemptProvision must use the injector backfilled via SetWorkloadInjector after construction")
}

// TestSetWorkloadInjector_NilInjectorIsNoOp guards SetWorkloadInjector's own
// documented no-op behavior for a nil argument — it must never overwrite an
// already-set injector with nothing.
func TestSetWorkloadInjector_NilInjectorIsNoOp(t *testing.T) {
	var recorded repositories.AgentThunderAttemptUpdate
	repo, resolver, store := successfulProvisionMocks(&recorded)
	reconciledCalls := 0
	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(context.Context, string, string, string, string) error {
			reconciledCalls++
			return nil
		},
	})

	setter, ok := svc.(WorkloadInjectorSetter)
	require.True(t, ok)
	setter.SetWorkloadInjector(nil)

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	})

	assert.Equal(t, 1, reconciledCalls, "SetWorkloadInjector(nil) must not clear an already-configured injector")
}

// TestSetWorkloadInjector_ConcurrentWithAttemptProvision_NoRace guards the
// workloadInjectorMu doc comment's safety claim: SetWorkloadInjector and the
// AttemptProvision calls that read the injector through getWorkloadInjector
// can genuinely run concurrently in production (app.Run's backfill happens
// from the main goroutine while a fast-path AttemptProvision from an
// in-flight request could already be running) and must never race. Meaningful
// only under `go test -race`.
func TestSetWorkloadInjector_ConcurrentWithAttemptProvision_NoRace(t *testing.T) {
	const n = 20
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "thunder-agent-1", "client-abc", "secret-xyz", true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc:    func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error { return nil },
	}
	svc := newTestProvisioningService(repo, resolver, store)
	setter, ok := svc.(WorkloadInjectorSetter)
	require.True(t, ok)

	injector := &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(context.Context, string, string, string, string) error { return nil },
	}

	var wg sync.WaitGroup
	wg.Add(1 + n)
	go func() {
		defer wg.Done()
		setter.SetWorkloadInjector(injector)
	}()
	for i := range n {
		go func(i int) {
			defer wg.Done()
			svc.AttemptProvision(context.Background(), models.AgentThunderClient{
				ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: fmt.Sprintf("agent-%d", i),
				EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
			})
		}(i)
	}
	wg.Wait()
}

func TestAttemptProvision_Success_ExternalAgent_SkipsWorkloadInjection(t *testing.T) {
	var recorded repositories.AgentThunderAttemptUpdate
	repo, resolver, store := successfulProvisionMocks(&recorded)

	// All stub funcs nil: any injector call panics, proving external agents
	// never reach the Gateway Binding hook.
	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, &agentIdentityInjectorStub{})

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
	})

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
}

// TestReconcileWorkloadInjection_NoInjectorConfigured_LogsWarningInsteadOfSilentSkip
// guards the startup-order contract between app.Run's SetWorkloadInjector
// backfill and this service: if that backfill is ever skipped or reordered
// (the injector stays nil in production), credential injection must not
// silently no-op — it must be loud in the logs.
func TestReconcileWorkloadInjection_NoInjectorConfigured_LogsWarningInsteadOfSilentSkip(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	reconcileWorkloadInjection(context.Background(), nil, models.AgentThunderClient{
		AgentName: "my-agent", EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	}, logger)

	logged := buf.String()
	assert.Contains(t, logged, "level=WARN")
	assert.Contains(t, logged, "no workload injector configured")
}

func TestReconcileWorkloadInjection_ExternalAgent_NoInjectorConfigured_NoWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	reconcileWorkloadInjection(context.Background(), nil, models.AgentThunderClient{
		AgentName: "my-agent", EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
	}, logger)

	assert.Empty(t, buf.String(), "external agents are never injected regardless of injector configuration — no warning expected")
}

func TestAttemptProvision_InjectorFailure_DoesNotAffectCompletion(t *testing.T) {
	var recorded repositories.AgentThunderAttemptUpdate
	repo, resolver, store := successfulProvisionMocks(&recorded)

	injector := &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(_ context.Context, _, _, _, _ string) error {
			return errors.New("workload reconcile failed")
		},
	}
	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, injector)

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	})

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status,
		"workload reconcile is best-effort — a failure must never un-complete the binding")
	require.NotNil(t, recorded.ThunderAgentID)
	assert.Equal(t, "thunder-agent-1", *recorded.ThunderAgentID)
}

func TestAttemptProvision_RecordFailure_SkipsInjection(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "", "", "", false, errors.New("thunder down")
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error {
			return nil
		},
	}
	// Nil stub funcs: an injector call on the failure path would panic.
	svc := newTestProvisioningServiceWithInjector(repo, resolver, &clientmocks.SecretManagementClientMock{}, &agentIdentityInjectorStub{})

	svc.AttemptProvision(context.Background(), models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeInternal,
	})
}

// TestDeleteAllBindings_DeletesStoredSecretsForBindingsThatHaveOne guards two
// separate cleanup paths at once: the stored credential
// (secretMgmtClient.DeleteSecret, which also deletes CreateSecret's
// auto-created SecretReference) for whichever binding actually has one, and
// the AMS-owned data-plane SecretReference (injector.CleanupForEnvironment)
// for every internal binding regardless of whether it has a stored secret —
// DeleteAllBindings runs that cleanup for internal bindings that never
// completed too, since a crash between storeCredential creating the
// SecretReference and UpdateAfterAttempt persisting SecretRefPath would
// otherwise leave it permanently orphaned. External bindings never have a
// stored secret (SecretRefPath is always empty for them) and never get the
// data-plane cleanup, since it's an internal-agent-only concept.
func TestDeleteAllBindings_DeletesStoredSecretsForBindingsThatHaveOne(t *testing.T) {
	internalBinding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "dev", ProvisioningType: models.AgentProvisioningTypeInternal,
		ThunderAgentID: "t-1", SecretRefPath: "p1",
	}
	externalBinding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "prod", ProvisioningType: models.AgentProvisioningTypeExternal,
		ThunderAgentID: "t-2", SecretRefPath: "", // external agents never have one stored
	}

	tc := fakeThunderClientMock()
	tc.DeleteAgentIdentityFunc = func(_ context.Context, _ string) (bool, error) { return true, nil }
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var deletedEnvs []string
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ string) error {
			deletedEnvs = append(deletedEnvs, location.EnvironmentName)
			return nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{internalBinding, externalBinding}, nil
		},
		GetFunc: func(_ context.Context, _, _, _, envName string) (*models.AgentThunderClient, error) {
			for _, b := range []models.AgentThunderClient{internalBinding, externalBinding} {
				if b.EnvironmentName == envName {
					return &b, nil
				}
			}
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		DeleteByIDsFunc:     func(_ context.Context, _ []uuid.UUID) error { return nil },
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}
	var cleanedUpEnvs []string
	injector := &agentIdentityInjectorStub{
		CleanupForEnvironmentFunc: func(_ context.Context, _, _, envName string) error {
			cleanedUpEnvs = append(cleanedUpEnvs, envName)
			return nil
		},
	}

	svc := newTestProvisioningServiceWithInjector(repo, resolver, store, injector)

	svc.DeleteAllBindings(context.Background(), "acme", "proj1", "my-agent")

	assert.Equal(t, []string{"dev"}, deletedEnvs,
		"the stored secret is deleted only for the binding that actually has one")
	assert.Equal(t, []string{"dev"}, cleanedUpEnvs,
		"the data-plane SecretReference is cleaned up for the internal binding but not the external one")
}

func TestDeleteAllBindings_ContinuesExternalCleanupWhenDBRowDeleteFails(t *testing.T) {
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "dev", ProvisioningType: models.AgentProvisioningTypeInternal,
		ThunderAgentID: "t-1", SecretRefPath: "p1",
	}

	tc := fakeThunderClientMock()
	thunderDeleted := false
	tc.DeleteAgentIdentityFunc = func(_ context.Context, _ string) (bool, error) {
		thunderDeleted = true
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	secretDeleted := false
	store := &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ string) error {
			secretDeleted = true
			return nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{binding}, nil
		},
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &binding, nil
		},
		DeleteByIDsFunc: func(_ context.Context, _ []uuid.UUID) error {
			return errors.New("db unavailable")
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}
	svc := newTestProvisioningService(repo, resolver, store)

	svc.DeleteAllBindings(context.Background(), "acme", "proj1", "my-agent")

	assert.True(t, thunderDeleted, "a failed local DB row delete must not block deleting the live Thunder identity")
	assert.True(t, secretDeleted, "a failed local DB row delete must not block deleting the stored secret")
}

func TestAttemptProvision_SerializesWithRegenerateSecret(t *testing.T) {
	// Proves AttemptProvision and RegenerateSecret cannot interleave their
	// Thunder RegenerateAgentSecret + OpenBao Store calls for the same
	// binding: AttemptProvision's recovery branch holds the binding lock for
	// its full duration, so a concurrent RegenerateSecret call must wait
	// until it releases.
	const org, proj, agent, env = "acme", "proj1", "my-agent", "staging"

	attemptEnteredThunderCall := make(chan struct{})
	releaseAttempt := make(chan struct{})

	// RegenerateAgentSecretFunc is shared by both AttemptProvision's recovery
	// branch (the first call, which must block to prove serialization) and
	// RegenerateSecret's own call (a later call, which only happens AFTER
	// AttemptProvision has released the binding lock and returned — so it
	// must NOT re-block or re-close the already-closed signaling channel).
	var callCount int32
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		if atomic.AddInt32(&callCount, 1) == 1 {
			close(attemptEnteredThunderCall)
			<-releaseAttempt // block here until the test says go, holding the binding lock the whole time
			return "recovered-secret", nil
		}
		return "regenerated-by-user", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return "ref", nil
		},
	}
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: org, ProjectName: proj, AgentName: agent,
		EnvironmentName: env, ProvisioningType: models.AgentProvisioningTypeInternal,
		ThunderAgentID: "already-created", ThunderClientID: "client-abc", SecretRefPath: "", // triggers the recovery branch
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error {
			return nil
		},
		// GetFunc/UpdateSecretRefFunc are needed for the RegenerateSecret call
		// this test also makes.
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &binding, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}
	svc := newTestProvisioningService(repo, resolver, store)

	go svc.AttemptProvision(context.Background(), binding)
	<-attemptEnteredThunderCall // AttemptProvision now holds the binding lock

	regenerateReturned := make(chan struct{})
	go func() {
		defer close(regenerateReturned)
		_, _, _, _ = svc.RegenerateSecret(context.Background(), org, proj, agent, env)
	}()

	select {
	case <-regenerateReturned:
		t.Fatal("RegenerateSecret must block while AttemptProvision holds the binding lock, but it returned immediately")
	case <-time.After(100 * time.Millisecond):
		// expected: still blocked
	}

	close(releaseAttempt)
	select {
	case <-regenerateReturned:
		// expected: unblocks once AttemptProvision releases the lock
	case <-time.After(2 * time.Second):
		t.Fatal("RegenerateSecret never unblocked after AttemptProvision released the binding lock")
	}
}

// TestHealSecretRef_CorrectsStaleRow guards the one-time backfill: a binding
// whose SecretRefPath still holds the old locally-computed KV-path guess
// (from before storeCredential was fixed) gets corrected to the real remote
// KV key, read back through GetSecretReference — never recomputed locally.
// The secret management mock has every func nil, so a CreateSecret or
// DeleteSecret call would panic: healing is a read plus one DB write.
func TestHealSecretRef_CorrectsStaleRow(t *testing.T) {
	svc := newTestProvisioningService(
		&repomocks.AgentThunderClientRepositoryMock{},
		&clientmocks.EnvThunderResolverMock{},
		&clientmocks.SecretManagementClientMock{},
	)
	impl := svc.(*agentThunderProvisioningService)

	refName := agentIdentitySecretLocation("acme", "proj1", "my-agent", "staging").SecretRefName()
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeInternal,
		SecretRefPath:    "acme/proj1/staging/my-agent/my-agent-agent-identity", // the old, wrong KV-path-shaped guess
	}

	var updatedID uuid.UUID
	var updatedPath string
	impl.repo = &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			// The fresh re-read taken under the binding lock — still stale,
			// matching the snapshot, so the write is expected to proceed.
			return &binding, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, id uuid.UUID, secretRefPath string) error {
			updatedID = id
			updatedPath = secretRefPath
			return nil
		},
	}

	require.NoError(t, impl.HealSecretRef(context.Background(), binding))
	assert.Equal(t, binding.ID, updatedID)
	assert.Equal(t, "resolved/"+refName, updatedPath, "must heal to the value resolved via GetSecretReference, never a locally-computed name")
}

// TestHealSecretRef_RevokedAfterSnapshot_DoesNotResurrectCredential guards the
// race a background startup sweep is exposed to: if a RevokeSecret or
// DeleteAllBindings call clears SecretRefPath to "" for this binding between
// the sweep's initial snapshot and this call's own GetSecretReference resolve
// completing, the resolved (but now orphaned) path must never be written back
// over the fresh, empty value — that would resurrect a revoked/deleted
// credential for the injection reconciler to re-inject.
func TestHealSecretRef_RevokedAfterSnapshot_DoesNotResurrectCredential(t *testing.T) {
	svc := newTestProvisioningService(
		&repomocks.AgentThunderClientRepositoryMock{},
		&clientmocks.EnvThunderResolverMock{},
		&clientmocks.SecretManagementClientMock{},
	)
	impl := svc.(*agentThunderProvisioningService)

	staleBinding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeInternal,
		SecretRefPath:    "acme/proj1/staging/my-agent/my-agent-agent-identity", // the sweep's stale snapshot
	}

	impl.repo = &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			// A concurrent revoke/delete landed and cleared it since the snapshot.
			revoked := staleBinding
			revoked.SecretRefPath = ""
			return &revoked, nil
		},
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error {
			t.Fatal("must not write a resolved path back over a binding that was revoked/cleared since the snapshot")
			return nil
		},
	}

	require.NoError(t, impl.HealSecretRef(context.Background(), staleBinding))
}

// TestHealSecretRef_DeletedAfterSnapshot_NoOp guards the sibling case: the
// binding row was deleted entirely (agent deletion) between the sweep's
// snapshot and this call's resolve completing.
func TestHealSecretRef_DeletedAfterSnapshot_NoOp(t *testing.T) {
	svc := newTestProvisioningService(
		&repomocks.AgentThunderClientRepositoryMock{},
		&clientmocks.EnvThunderResolverMock{},
		&clientmocks.SecretManagementClientMock{},
	)
	impl := svc.(*agentThunderProvisioningService)

	staleBinding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeInternal,
		SecretRefPath:    "acme/proj1/staging/my-agent/my-agent-agent-identity",
	}

	impl.repo = &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error {
			t.Fatal("must not write for a binding deleted since the snapshot")
			return nil
		},
	}

	require.NoError(t, impl.HealSecretRef(context.Background(), staleBinding))
}

// TestHealSecretRef_NoOpWhenAlreadyCorrect guards against a needless DB write
// on every restart once a binding has already been healed (or was always
// correct, e.g. provisioned after the fix shipped).
func TestHealSecretRef_NoOpWhenAlreadyCorrect(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error {
			t.Fatal("must not write when the stored ref is already correct")
			return nil
		},
	}
	svc := newTestProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, &clientmocks.SecretManagementClientMock{})
	impl := svc.(*agentThunderProvisioningService)

	refName := agentIdentitySecretLocation("acme", "proj1", "my-agent", "staging").SecretRefName()
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeInternal,
		SecretRefPath:    "resolved/" + refName, // already the resolved value HealSecretRef would derive
	}

	require.NoError(t, impl.HealSecretRef(context.Background(), binding))
}

// TestHealSecretRef_SkipsExternalAndRevoked guards the two states that must
// never be touched: external agents never have a stored secret at all, and a
// revoked binding's empty SecretRefPath must stay empty, not be "healed"
// into a name pointing at a secret that no longer exists.
func TestHealSecretRef_SkipsExternalAndRevoked(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error {
			t.Fatal("must not write for an external agent or a revoked (empty SecretRefPath) binding")
			return nil
		},
	}
	svc := newTestProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, &clientmocks.SecretManagementClientMock{})
	impl := svc.(*agentThunderProvisioningService)

	external := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeExternal,
		SecretRefPath:    "acme/proj1/staging/my-agent/my-agent-agent-identity",
	}
	revoked := models.AgentThunderClient{
		ID: uuid.New(), OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeInternal,
		SecretRefPath:    "",
	}

	require.NoError(t, impl.HealSecretRef(context.Background(), external))
	require.NoError(t, impl.HealSecretRef(context.Background(), revoked))
}
